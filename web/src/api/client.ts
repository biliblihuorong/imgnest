/**
 * 同源 HTTP 客户端：自动携带 Bearer token、解析 {code,message,data} 外壳、
 * 统一错误与 401/20001/20002 未授权回调。
 *
 * 注意：本模块不得 import 任何 store（避免循环依赖）；token 只读
 * localStorage["imgnest.token"]，由 stores/auth 负责写入与清除。
 */

export const TOKEN_STORAGE_KEY = "imgnest.token";

/** 业务错误：code 为外壳业务码（网络/解析失败为 -1），status 为 HTTP 状态码（网络失败为 0）。 */
export class ApiError extends Error {
  readonly code: number;
  readonly status: number;

  constructor(code: number, message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.status = status;
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
 * - 网络失败或响应体不是 JSON：抛 ApiError{code: -1, message: "网络错误", status: 0}；
 * - 业务码 20001（未鉴权/凭证失效）：先触发 setUnauthorizedHandler 注册的回调再抛错。
 */
export async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  let response: Response;
  try {
    const headers = new Headers(init.headers ?? undefined);
    const token = getToken();
    if (token) {
      headers.set("Authorization", `Bearer ${token}`);
    }
    response = await fetch(path, { ...init, headers });
  } catch {
    throw new ApiError(-1, "网络错误", 0);
  }

  let envelope: Envelope;
  try {
    envelope = (await response.json()) as Envelope;
  } catch {
    throw new ApiError(-1, "网络错误", 0);
  }

  const code = typeof envelope.code === "number" ? envelope.code : -1;
  const message = typeof envelope.message === "string" ? envelope.message : "未知错误";

  if (code !== 0) {
    notifyUnauthorized(code);
    throw new ApiError(code, message, response.status);
  }
  return envelope.data as T;
}

function notifyUnauthorized(code: number): void {
  // 仅凭证本身失效（20001：过期/吊销/用户禁用）才清会话；
  // 20002 是"凭证内容错误"（如改密时旧密码写错），token 仍有效，只把错误抛给调用方。
  if (code !== 20001) {
    return;
  }
  unauthorizedHandler?.();
}
