/**
 * 同源 HTTP 客户端：自动携带 Bearer token、解析 {code,message,data} 外壳、
 * 统一错误与 401/20001/20002 未授权回调。
 *
 * 注意：本模块不得 import 任何 store（避免循环依赖）；token 只读
 * localStorage["imgnest.token"]，由 stores/auth 负责写入与清除。
 */

export const TOKEN_STORAGE_KEY = "imgnest.token";

/**
 * 业务错误：code 为外壳业务码（网络/解析失败为 -1），status 为 HTTP 状态码
 * （仅网络失败为 0；响应体不是 JSON 时保留真实 HTTP 状态）。
 */
export class ApiError extends Error {
  readonly code: number;
  readonly status: number;
  readonly data: unknown;

  constructor(code: number, message: string, status: number, data?: unknown) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.status = status;
    this.data = data;
  }
}

export type UnauthorizedHandler = () => void;

let unauthorizedHandler: UnauthorizedHandler | null = null;

/** 注册未授权回调（由 main.ts 注册为「清空登录态并跳转 /login」）。传 null 注销。 */
export function setUnauthorizedHandler(handler: UnauthorizedHandler | null): void {
  unauthorizedHandler = handler;
}

/** 读取当前 bearer token；无 token 返回 null。 */
export function getToken(): string | null {
  try {
    return localStorage.getItem(TOKEN_STORAGE_KEY);
  } catch {
    return null;
  }
}

interface Envelope {
  code?: unknown;
  message?: unknown;
  data?: unknown;
}

/**
 * 发起同源请求并解包 data。
 * - 外壳 code !== 0：抛 ApiError{code, message, status}；
 * - 网络失败：抛 ApiError{code: -1, message: "网络错误", status: 0}；
 * - 响应体不是 JSON（如反代/网关的错误页）：抛 ApiError{code: -1, message: "网络错误",
 *   status: 真实 HTTP 状态}；其中 status === 401 视同 20001 先触发未授权回调
 *   （携带我们的 Bearer 却被网关拒绝，会话已不可用，与 XHR 上传通道同语义）；
 * - 业务码 20001（未鉴权/凭证失效）：先触发 setUnauthorizedHandler 注册的回调再抛错。
 */
export async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const token = getToken();
  let response: Response;
  try {
    const headers = new Headers(init.headers ?? undefined);
    if (token) {
      headers.set("Authorization", `Bearer ${token}`);
    }
    response = await fetch(path, { ...init, headers });
  } catch (error) {
    if (init.signal?.aborted || (error instanceof DOMException && error.name === "AbortError"))
      throw error;
    throw new ApiError(-1, "网络错误", 0);
  }

  let envelope: Envelope;
  try {
    envelope = (await response.json()) as Envelope;
  } catch (error) {
    if (init.signal?.aborted || (error instanceof DOMException && error.name === "AbortError"))
      throw error;
    // 非 JSON 响应：保留真实 HTTP 状态。401 说明「携带我们的 Bearer 却被反代/网关拒绝」，
    // 会话已失效，按 20001 同语义清会话（同 XHR 上传通道 parseUploadResponse）。
    if (response.status === 401) {
      notifyUnauthorized(20001, token, init.signal);
    }
    throw new ApiError(-1, "网络错误", response.status);
  }

  const code = typeof envelope.code === "number" ? envelope.code : -1;
  const message = typeof envelope.message === "string" ? envelope.message : "未知错误";

  if (code !== 0) {
    notifyUnauthorized(code, token, init.signal);
    throw new ApiError(code, message, response.status, envelope.data);
  }
  return envelope.data as T;
}

/**
 * 会话失效通知：仅凭证本身失效（20001：过期/吊销/用户禁用）才触发回调，
 * 清态跳登录；20002 是"凭证内容错误"（如改密时旧密码写错），token 仍有效，
 * 只把错误抛给调用方。request() 内部与 XHR 上传通道共用。
 */
export function notifyUnauthorized(
  code: number,
  requestToken: string | null,
  signal?: AbortSignal | null,
): void {
  // Late responses must never clear a newer login or an already-cancelled view.
  if (code !== 20001 || signal?.aborted || getToken() !== requestToken) {
    return;
  }
  unauthorizedHandler?.();
}
