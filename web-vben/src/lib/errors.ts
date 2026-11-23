import { $t } from "@vben/locales";
import { ApiError } from "@/api/client";
import { describeError } from "@/app.hooks";

/**
 * 把接口错误转成给人看的文案：先找 errors.<code> 的翻译，没有就用通用文案。
 * 后端返回的 message 不直接展示，避免把内部细节暴露给用户；需要展示的在 app.hooks.ts 的 describeError 里放行。
 */
export function formatApiError(error: unknown, fallback = "common.errors.unknown"): string {
  const custom = describeError(error);
  if (custom) return custom;
  if (error instanceof ApiError) {
    if (error.code === -1) return $t("common.errors.network");
    const key = `common.errors.${error.code}`;
    const text = $t(key);
    if (text !== key) return text;
  }
  return $t(fallback);
}
