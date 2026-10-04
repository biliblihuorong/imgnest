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
import { ApiError, getToken, notifyUnauthorized } from "./client";
import type { components } from "./schema";

export type ImageView = components["schemas"]["ImageView"];
export type ImageLinks = components["schemas"]["ImageLinks"];
export type ImageBatchItem = components["schemas"]["ImageBatchItem"];

/**
 * 客户端预检限额，与服务端一致：
 * docs/openapi.yaml /api/upload（"Defaults: 64 MiB total request, at most 20
 * files"）与 deploy/config.example.yaml server.max_upload_mb=20、
 * server.max_request_mb=64。超限直接拒绝，不发请求。
 */
export const MAX_UPLOAD_FILES = 20;
export const MAX_UPLOAD_FILE_BYTES = 20 * 1024 * 1024; // server.max_upload_mb=20（MiB，1024 进制）
export const MAX_UPLOAD_TOTAL_BYTES = 64 * 1024 * 1024; // server.max_request_mb=64

/** 客户端预检结果：不通过时 errors 为可直接展示的中文提示。 */
export interface PreflightResult {
  ok: boolean;
  errors: string[];
}

/** 上传前的客户端预检：文件数 / 单文件字节 / 合计字节。 */
export function preflightUpload(files: readonly File[]): PreflightResult {
  const errors: string[] = [];
  if (files.length === 0) {
    errors.push("请先选择要上传的文件");
  }
  if (files.length > MAX_UPLOAD_FILES) {
    errors.push(`单次最多上传 ${MAX_UPLOAD_FILES} 个文件，当前选择了 ${files.length} 个`);
  }
  const oversized: string[] = [];
  let total = 0;
  for (const file of files) {
    total += file.size;
    if (file.size > MAX_UPLOAD_FILE_BYTES) {
      oversized.push(`「${file.name}」(${formatBytes(file.size)})`);
    }
  }
  if (oversized.length > 0) {
    errors.push(`单文件上限 ${formatBytes(MAX_UPLOAD_FILE_BYTES)}，超限文件：${oversized.join("、")}`);
  }
  if (total > MAX_UPLOAD_TOTAL_BYTES) {
    errors.push(`单次合计上限 ${formatBytes(MAX_UPLOAD_TOTAL_BYTES)}，本次共 ${formatBytes(total)}`);
  }
  return { ok: errors.length === 0, errors };
}

/** 单个文件的上传结果（对应 openapi 201 data=ImageView 与 207 data=ImageBatchItem[]）。 */
export type UploadItemResult =
  | { ok: true; filename: string; status: number; image: ImageView }
  | { ok: false; filename: string; status: number; code: number; message: string };

export interface UploadImageOptions {
  policyId: number | null;
  isPublic: boolean;
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
  const results: UploadItemResult[] = [];
  for (const [index, file] of options.files.entries()) {
    options.onFileStart?.(index, file);
    // 把批量回调接到单文件 XHR 的 upload.onprogress 上
    const onProgress = options.onFileProgress
      ? (loaded: number, total: number) => options.onFileProgress?.(index, file, loaded, total)
      : options.onProgress;
    const result = await uploadOne(file, {
      policyId: options.policyId,
      isPublic: options.isPublic,
      onProgress,
    });
    results.push(result);
    options.onFileSettled?.(index, file, result);
  }
  return results;
}

async function uploadOne(file: File, options: UploadImageOptions): Promise<UploadItemResult> {
  try {
    const { image, status } = await requestUpload(
      file,
      options.policyId,
      options.isPublic,
      options.onProgress,
    );
    return { ok: true, filename: file.name, status, image };
  } catch (error) {
    const apiError =
      error instanceof ApiError ? error : new ApiError(-1, "网络错误", 0);
    return {
      ok: false,
      filename: file.name,
      status: apiError.status,
      code: apiError.code,
      message: apiError.message,
    };
  }
}

/** 发起单文件上传 XHR；失败抛 ApiError（与 client 一致的错误语义）。 */
function requestUpload(
  file: File,
  policyId: number | null,
  isPublic: boolean,
  onProgress?: (loaded: number, total: number) => void,
): Promise<{ image: ImageView; status: number }> {
  return new Promise((resolve, reject) => {
    const form = new FormData();
    form.append("file", file, file.name);
    if (policyId !== null) {
      form.append("policy_id", String(policyId));
    }
    // openapi：multipart 文本必须精确为 true / false
    form.append("is_public", isPublic ? "true" : "false");

    const xhr = new XMLHttpRequest();
    xhr.open("POST", "/api/upload");
    const token = getToken();
    if (token) {
      // token 只进 Authorization 头，不进 URL、不进日志
      xhr.setRequestHeader("Authorization", `Bearer ${token}`);
    }
    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable) {
        onProgress?.(event.loaded, event.total);
      }
    };
    xhr.onload = () => {
      try {
        resolve(parseUploadResponse(xhr));
      } catch (error) {
        reject(error);
      }
    };
    xhr.onerror = () => reject(new ApiError(-1, "网络错误", 0));
    xhr.send(form);
  });
}

interface Envelope {
  code?: unknown;
  message?: unknown;
  data?: unknown;
}

function parseUploadResponse(xhr: XMLHttpRequest): { image: ImageView; status: number } {
  const status = xhr.status;
  let envelope: Envelope;
  try {
    envelope = JSON.parse(xhr.responseText) as Envelope;
  } catch {
    throw new ApiError(-1, "网络错误", 0);
  }
  const code = typeof envelope.code === "number" ? envelope.code : -1;
  const message = typeof envelope.message === "string" ? envelope.message : "未知错误";
  if (code !== 0) {
    notifyUnauthorized(code);
    throw new ApiError(code, message, status);
  }
  if (status === 207) {
    // 批量响应：data 为逐文件结果；单文件请求只取第一项
    const items = envelope.data as ImageBatchItem[] | undefined;
    const item = Array.isArray(items) ? items[0] : undefined;
    if (!item) {
      throw new ApiError(-1, "上传响应缺少结果项", status);
    }
    if (item.code !== 0 || !isImageView(item.data)) {
      notifyUnauthorized(item.code || -1);
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
