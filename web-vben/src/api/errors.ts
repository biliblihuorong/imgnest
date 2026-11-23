/**
 * 业务错误：code 为外壳业务码（网络/解析失败为 -1），status 为 HTTP 状态码（网络失败为 0）。
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
