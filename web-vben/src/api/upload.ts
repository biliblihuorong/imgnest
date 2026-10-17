/**
 * 上传 API：POST /api/upload（multipart）。
 *
 * 为什么不用 client.request：fetch 不暴露上传进度，这里用 XHR 逐文件发起
 * 请求，每个文件独立一条 HTTP 事务（真实逐文件字节进度 + 逐文件补偿语义，
 * 服务端默认并发为 2，顺序发送最保守）。响应解析与 client 保持一致：
 * 解 {code,message,data} 外壳，code !== 0 / 非 JSON / 网络失败抛
 * ApiError{code,message,status}（网络失败 code=-1、status=0）。
 *
 * 单文件请求按 openapi 语义返回 201（data=ImageView）；解析层同时兼容
 * 207（data=ImageBatchItem[]，取对应项），调用方拿到的统一是逐文件结果。
 */

import { formatBytes } from "@/lib/format";
import { ApiError, getToken, notifyUnauthorized, request } from "./client";
import type { components } from "./schema";

export type ImageView = components["schemas"]["ImageView"];
export type ImageLinks = components["schemas"]["ImageLinks"];
export type ImageBatchItem = components["schemas"]["ImageBatchItem"];
export type UploadLimits = components["schemas"]["UploadLimits"];

/**
 * 客户端预检：单次最多选 20 个文件（与 /api/upload 的 max_files 一致）。
 * 单文件上限按用户组而定、没有固定值，从 GET /api/upload/limits 读取；
 * 读不到时不在客户端拦，交给服务端判断。每个文件单独一个请求，所以
 * 没有「合计」上限。超限直接拒绝，不发请求。
 */
export const MAX_UPLOAD_FILES = 20;

/** 当前用户的上传限额（单文件上限按用户组，可大于 20 MiB）。 */
export function getUploadLimits(): Promise<UploadLimits> {
  return request<UploadLimits>("/api/upload/limits");
}

/** 游客组的上传限额；游客上传关闭时为 401。 */
export function getGuestUploadLimits(): Promise<UploadLimits> {
  return request<UploadLimits>("/api/guest/upload/limits");
}

/** 客户端预检结果：不通过时 errors 为可直接展示的中文提示。 */
export interface PreflightResult {
  ok: boolean;
  errors: string[];
}

/**
 * 上传前的客户端预检：文件数 / 单文件字节。maxFileBytes 为 null（限额
 * 还没读到）时不检查单文件大小。
 */
export function preflightUpload(
  files: readonly File[],
  maxFileBytes: number | null = null,
): PreflightResult {
  const errors: string[] = [];
  if (files.length === 0) {
    errors.push("请先选择要上传的文件");
  }
  if (files.length > MAX_UPLOAD_FILES) {
    errors.push(`单次最多上传 ${MAX_UPLOAD_FILES} 个文件，当前选择了 ${files.length} 个`);
  }
  const oversized: string[] = [];
  for (const file of files) {
    if (maxFileBytes !== null && file.size > maxFileBytes) {
      oversized.push(`「${file.name}」(${formatBytes(file.size)})`);
    }
  }
  if (maxFileBytes !== null && oversized.length > 0) {
    errors.push(`单文件上限 ${formatBytes(maxFileBytes)}，超限文件：${oversized.join("、")}`);
  }
  return { ok: errors.length === 0, errors };
}

/** 单个文件的上传结果（对应 openapi 201 data=ImageView 与 207 data=ImageBatchItem[]）。 */
export type UploadItemResult =
  | { ok: true; filename: string; status: number; image: ImageView }
  | {
      ok: false;
      filename: string;
      status: number;
      code: number;
      message: string;
      /** 错误外壳的 data（可能带 reason 提示码）。 */
      data?: unknown;
    };

export interface UploadImageOptions {
  policyId: number | null;
  /**
   * 游客上传（公开图床页）：走 POST /api/guest/upload，不带 Token、不发
   * album_id / is_public，401 也不触发登出。
   */
  guest?: boolean;
  isPublic: boolean;
  /**
   * 目标相册 id；null/undefined = 不入相册（不发送 album_id 字段）。
   * openapi /api/upload multipart 契约的 album_id（M2 起后端已支持）。
   */
  albumId?: number | null;
  /** Cancels the current request and all remaining files in this batch. */
  signal?: AbortSignal;
  /** 单文件的 XHR upload.onprogress（字节进度）。 */
  onProgress?: (loaded: number, total: number) => void;
}

export interface UploadImagesOptions extends UploadImageOptions {
  files: File[];
  /** 第 index 个文件开始上传。 */
  onFileStart?: (index: number, file: File) => void;
  /** 第 index 个文件的字节进度。 */
  onFileProgress?: (index: number, file: File, loaded: number, total: number) => void;
  /** 第 index 个文件上传结束（成功或失败，均不抛出）。 */
  onFileSettled?: (index: number, file: File, result: UploadItemResult) => void;
}

/**
 * 顺序逐文件上传。任何失败都归一为该文件的失败项（ApiError 语义的
 * code/message/status），整体不抛出；返回结果顺序与 files 一致。
 */
export async function uploadImages(options: UploadImagesOptions): Promise<UploadItemResult[]> {
  // A batch belongs to the session that started it, including guest sessions.
  // Guest uploads never send a token, so a later sign-in does not cancel them.
  const token = options.guest ? null : getToken();
  const controller = new AbortController();
  const abort = () => controller.abort();
  options.signal?.addEventListener("abort", abort, { once: true });
  window.addEventListener("imgnest:session-cleared", abort);
  if (options.signal?.aborted) abort();
  const results: UploadItemResult[] = [];
  try {
    const sessionChanged = () => !options.guest && getToken() !== token;
    for (const [index, file] of options.files.entries()) {
      if (sessionChanged()) abort();
      if (!controller.signal.aborted) options.onFileStart?.(index, file);
      // A callback may clear the session before the request is constructed.
      if (sessionChanged()) abort();
      const onProgress = options.onFileProgress
        ? (loaded: number, total: number) => options.onFileProgress?.(index, file, loaded, total)
        : options.onProgress;
      const result = await uploadOne(
        file,
        {
          policyId: options.policyId,
          isPublic: options.isPublic,
          albumId: options.albumId,
          guest: options.guest,
          signal: controller.signal,
          onProgress,
        },
        token,
      );
      // Authentication failure ends the batch; ordinary file failures still continue.
      if (!result.ok && (result.status === 401 || result.code === 20001)) abort();
      results.push(result);
      options.onFileSettled?.(index, file, result);
    }
    return results;
  } finally {
    options.signal?.removeEventListener("abort", abort);
    window.removeEventListener("imgnest:session-cleared", abort);
  }
}

async function uploadOne(
  file: File,
  options: UploadImageOptions,
  token: string | null,
): Promise<UploadItemResult> {
  try {
    const guest = options.guest === true;
    const { image, status } = await requestUpload(
      file,
      options.policyId,
      options.isPublic,
      options.albumId ?? null,
      token,
      guest,
      options.signal,
      options.onProgress,
    );
    if (options.signal?.aborted || (!guest && getToken() !== token)) throw uploadAborted();
    return { ok: true, filename: file.name, status, image };
  } catch (error) {
    const apiError = error instanceof ApiError ? error : new ApiError(-1, "网络错误", 0);
    return {
      ok: false,
      filename: file.name,
      status: apiError.status,
      code: apiError.code,
      message: apiError.message,
      data: apiError.data,
    };
  }
}

function uploadAborted(): ApiError {
  return new ApiError(-1, "上传已取消", 0);
}

/** 发起单文件上传 XHR；失败抛 ApiError（与 client 一致的错误语义）。 */
function requestUpload(
  file: File,
  policyId: number | null,
  isPublic: boolean,
  albumId: number | null,
  token: string | null,
  guest: boolean,
  signal?: AbortSignal,
  onProgress?: (loaded: number, total: number) => void,
): Promise<{ image: ImageView; status: number }> {
  const sessionChanged = () => !guest && getToken() !== token;
  return new Promise((resolve, reject) => {
    if (signal?.aborted || sessionChanged()) {
      reject(uploadAborted());
      return;
    }
    const form = new FormData();
    form.append("file", file, file.name);
    if (policyId !== null) {
      form.append("policy_id", String(policyId));
    }
    // 游客不能选相册，可见性由服务端固定为私有，两个字段都不发。
    if (!guest) {
      // openapi：multipart 文本必须精确为 true / false
      form.append("is_public", isPublic ? "true" : "false");
      if (albumId !== null) {
        form.append("album_id", String(albumId));
      }
    }

    const xhr = new XMLHttpRequest();
    let settled = false;
    function cleanup() {
      settled = true;
      signal?.removeEventListener("abort", abort);
      xhr.onload = null;
      xhr.onerror = null;
      xhr.onabort = null;
      xhr.upload.onprogress = null;
    }
    function fail(error: unknown) {
      if (settled) return;
      cleanup();
      reject(error);
    }
    function abort() {
      if (settled) return;
      cleanup();
      xhr.abort();
      reject(uploadAborted());
    }
    xhr.open("POST", guest ? "/api/guest/upload" : "/api/upload");
    if (token && !guest) {
      // token 只进 Authorization 头，不进 URL、不进日志
      xhr.setRequestHeader("Authorization", `Bearer ${token}`);
    }
    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable) {
        onProgress?.(event.loaded, event.total);
      }
    };
    xhr.onload = () => {
      if (settled) return;
      // Stop listening before parsing: a current-session 401 can synchronously
      // clear the session, but its original per-file error must be preserved.
      cleanup();
      try {
        if (signal?.aborted || sessionChanged()) throw uploadAborted();
        resolve(parseUploadResponse(xhr, token, signal, !guest));
      } catch (error) {
        reject(error);
      }
    };
    xhr.onerror = () => fail(new ApiError(-1, "网络错误", 0));
    xhr.onabort = () => fail(uploadAborted());
    signal?.addEventListener("abort", abort, { once: true });
    try {
      xhr.send(form);
    } catch (error) {
      fail(error);
    }
  });
}

interface Envelope {
  code?: unknown;
  message?: unknown;
  data?: unknown;
}

/** notify=false（游客上传）时 401 只是本文件失败，不清空登录态。 */
function parseUploadResponse(
  xhr: XMLHttpRequest,
  token: string | null,
  signal?: AbortSignal,
  notify = true,
): { image: ImageView; status: number } {
  const status = xhr.status;
  let envelope: Envelope;
  try {
    envelope = JSON.parse(xhr.responseText) as Envelope;
  } catch {
    // Even a proxy's non-JSON 401 must end the batch before another file is sent.
    throw new ApiError(-1, "网络错误", status === 401 ? status : 0);
  }
  const code = typeof envelope.code === "number" ? envelope.code : -1;
  const message = typeof envelope.message === "string" ? envelope.message : "未知错误";
  if (code !== 0) {
    if (notify) notifyUnauthorized(code, token, signal);
    throw new ApiError(code, message, status, envelope.data);
  }
  if (status === 207) {
    // 批量响应：data 为逐文件结果；单文件请求只取第一项
    const items = envelope.data as ImageBatchItem[] | undefined;
    const item = Array.isArray(items) ? items[0] : undefined;
    if (!item) {
      throw new ApiError(-1, "上传响应缺少结果项", status);
    }
    if (item.code !== 0 || !isImageView(item.data)) {
      if (notify) notifyUnauthorized(item.code || -1, token, signal);
      throw new ApiError(item.code || -1, item.message || "上传失败", item.status || status);
    }
    return { image: item.data as ImageView, status: item.status || status };
  }
  if (!isImageView(envelope.data)) {
    throw new ApiError(-1, "上传响应缺少图片数据", status);
  }
  return { image: envelope.data, status };
}

function isImageView(value: unknown): value is ImageView {
  return typeof value === "object" && value !== null && "key" in value && "links" in value;
}
