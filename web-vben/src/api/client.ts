/**
 * HTTP 客户端：自动携带 Bearer token，解析 {code, message, data} 外壳，
 * 统一抛 ApiError；凭证失效时调用 setUnauthorizedHandler 注册的回调。
 *
 * 后端约定（换后端时只改这里）：
 * - 成功：HTTP 2xx + {"code": 0, "data": ...}
 * - 业务错误：{"code": 非 0, "message": "..."}
 * - 凭证失效：code === UNAUTHORIZED_CODE；响应不是 JSON 时看 HTTP 401。
 *   带外壳的 401 以 code 为准（例如「当前密码不正确」也可能是 401，但会话仍有效）。
 *
 * token 统一由 lib/session.ts 提供（含跨标签同步与内存降级）。本模块不得 import 任何 store，避免循环依赖。
 */
import { getSessionToken, setSessionToken } from "@/lib/session";
import { ApiError } from "./errors";

export { ApiError };
export { TOKEN_STORAGE_KEY } from "@/lib/session";

export const UNAUTHORIZED_CODE = 20001;

let unauthorizedHandler: (() => void) | null = null;

/** 由 bootstrap 注册为「清空登录态并跳转登录页」。传 null 注销。 */
export function setUnauthorizedHandler(handler: (() => void) | null): void {
  unauthorizedHandler = handler;
}

export function getToken(): string | null {
  return getSessionToken();
}

/** 直接改写会话（测试与特殊流程用）。登录、退出请走 stores/auth.ts。 */
export function setToken(token: string | null): void {
  setSessionToken(token);
}

interface Envelope {
  code?: unknown;
  message?: unknown;
  data?: unknown;
}

/**
 * 会话失效通知：只有 code === UNAUTHORIZED_CODE 才触发回调（清登录态并跳登录页）。
 * 迟到的响应（请求已取消，或发请求时的 token 已经不是当前 token）不能清掉之后新建立的会话。
 * request()、requestBlob() 内部使用；项目自己的 XHR 通道（例如带进度的上传）也应调用它。
 */
export function notifyUnauthorized(
  code: number,
  requestToken: string | null,
  signal?: AbortSignal | null,
): void {
  if (code !== UNAUTHORIZED_CODE || signal?.aborted || getToken() !== requestToken) return;
  unauthorizedHandler?.();
}

function isAbort(error: unknown, signal?: AbortSignal | null): boolean {
  return Boolean(signal?.aborted) || (error instanceof DOMException && error.name === "AbortError");
}

/** 只有普通对象和数组才自动 JSON 序列化；FormData、Blob、URLSearchParams、字符串等原样发送。 */
function isJsonBody(body: unknown): boolean {
  if (Array.isArray(body)) return true;
  if (body === null || typeof body !== "object") return false;
  const proto = Object.getPrototypeOf(body) as unknown;
  return proto === Object.prototype || proto === null;
}

/** 发起请求并解包 data。body 是普通对象或数组时自动 JSON 序列化。 */
export async function request<T>(
  path: string,
  init: Omit<RequestInit, "body"> & { body?: unknown } = {},
): Promise<T> {
  const token = getToken();
  const headers = new Headers(init.headers ?? undefined);
  if (token) headers.set("Authorization", `Bearer ${token}`);
  let body = init.body as BodyInit | null | undefined;
  if (isJsonBody(init.body)) {
    headers.set("Content-Type", "application/json");
    body = JSON.stringify(init.body);
  }

  // 条件必须和 ./mode.ts 的 MOCK_MODE 一致，且原地直接读 import.meta.env：构建时被替换成字面量，
  // 关闭时这个分支连同 mock.ts（演示账号、假数据）不会出现在产物里。跨模块引用常量做不到这一点。
  if (
    import.meta.env.DEV
      ? import.meta.env.VITE_USE_MOCK !== "false"
      : import.meta.env.VITE_USE_MOCK === "true"
  ) {
    try {
      const { mockRequest } = await import("./mock");
      return (await mockRequest(init.method ?? "GET", path, init.body, token)) as T;
    } catch (error) {
      if (error instanceof ApiError) notifyUnauthorized(error.code, token, init.signal);
      throw error;
    }
  }

  let response: Response;
  try {
    response = await fetch(path, { ...init, headers, body });
  } catch (error) {
    if (isAbort(error, init.signal)) throw error;
    throw new ApiError(-1, "network", 0);
  }

  let envelope: Envelope;
  try {
    envelope = (await response.json()) as Envelope;
  } catch (error) {
    if (isAbort(error, init.signal)) throw error;
    // 非 JSON 响应（反代/网关的错误页）：保留真实 HTTP 状态。401 说明带着我们的 token
    // 却被网关拒绝，会话已不可用，按凭证失效处理。
    if (response.status === 401) notifyUnauthorized(UNAUTHORIZED_CODE, token, init.signal);
    throw new ApiError(-1, "network", response.status);
  }

  const code = typeof envelope.code === "number" ? envelope.code : -1;
  const message = typeof envelope.message === "string" ? envelope.message : "unknown";
  if (code !== 0) {
    notifyUnauthorized(code, token, init.signal);
    throw new ApiError(code, message, response.status, envelope.data);
  }
  return envelope.data as T;
}

/**
 * 下载非 JSON 的响应体（CSV 导出、私有图片、文件等），带同样的 token、取消与错误处理：
 * 失败响应按 {code, message} 外壳解析；不是 JSON 的 401 当作凭证失效。演示模式下不走假后端。
 */
export async function requestBlob(
  path: string,
  init: { signal?: AbortSignal } = {},
): Promise<Blob> {
  const token = getToken();
  const headers = new Headers();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  let response: Response;
  try {
    response = await fetch(path, { headers, signal: init.signal });
  } catch (error) {
    if (isAbort(error, init.signal)) throw error;
    throw new ApiError(-1, "network", 0);
  }
  if (response.ok && !response.headers.get("content-type")?.includes("application/json"))
    return response.blob();
  let envelope: Envelope = {};
  try {
    envelope = (await response.json()) as Envelope;
  } catch (error) {
    if (isAbort(error, init.signal)) throw error;
    // 也不是 JSON：按 HTTP 状态报错。
  }
  const code =
    typeof envelope.code === "number"
      ? envelope.code
      : response.status === 401
        ? UNAUTHORIZED_CODE
        : -1;
  notifyUnauthorized(code, token, init.signal);
  throw new ApiError(
    code,
    typeof envelope.message === "string" ? envelope.message : "network",
    response.status,
    envelope.data,
  );
}
