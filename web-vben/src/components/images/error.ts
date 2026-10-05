/** 把请求错误转成可展示文案（ApiError 有 message；其余兜底）。 */
export function apiErrorMessage(error: unknown): string {
  if (error instanceof Error && error.message) {
    return error.message;
  }
  return "未知错误";
}
