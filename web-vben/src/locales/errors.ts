import { $t, $te } from "@vben/locales";
import { ApiError } from "@/api/client";

/**
 * 服务端附带的原因代码（data.reason）对应的具体提示，例如「密码需要 12–72 个字符」
 * 或「存储拒绝了访问密钥」。未知代码返回 undefined，调用方退回通用文案。
 */
export function reasonText(reason: unknown): string | undefined {
  if (typeof reason !== "string" || !/^[a-z_]{1,48}$/.test(reason)) return undefined;
  const key = `common.errors.reasons.${reason}`;
  return $te(key) ? $t(key) : undefined;
}

function errorReason(error: ApiError): string | undefined {
  const data = error.data;
  if (data && typeof data === "object" && "reason" in data) {
    return reasonText((data as { reason: unknown }).reason);
  }
  return undefined;
}

/** Localize structured error codes, never display arbitrary backend diagnostics. */
export function formatApiError(error: unknown, fallbackKey = "common.errors.unknown"): string {
  if (!(error instanceof ApiError)) return $t(fallbackKey);
  const reason = errorReason(error);
  // 参数错误的原因本身就是完整提示；其他错误把原因接在通用文案后面。
  if (reason && error.code === 10001) return reason;
  const base = formatBaseError(error, fallbackKey);
  return reason ? $t("common.errors.withReason", { error: base, reason }) : base;
}

function formatBaseError(error: ApiError, fallbackKey: string): string {
  // 网关或反向代理返回的非 JSON 错误页：服务没有正常响应，不是用户的网络问题。
  if (error.code === -1 && error.status >= 500)
    return $t("common.errors.gateway", { status: error.status });
  // 10005 is a plugin setting the administrator must fix; its message is
  // written for them by the plugin and never carries diagnostics.
  if (error.code === 10005 && error.message) return error.message;
  let key: string | undefined;
  if (error.status === 404) key = "notFound";
  else if (error.code === 10001) key = error.status === 404 ? "notFound" : "invalid";
  else if (error.code === 20002)
    key = fallbackKey.toLowerCase().includes("password") ? "currentPassword" : "credentials";
  else
    key = (
      {
        [-1]: "network",
        10002: "tooLarge",
        10006: "notLskyDump",
        10004: "network",
        20001: "unauthorized",
        20003: "forbidden",
        30001: "registerDisabled",
        30002: "exists",
        30003: "rateLimit",
        30004: "quota",
        30005: "conflict",
        30006: "busy",
        30007: "format",
        30008: "groupMembers",
        30009: "referenced",
        30010: "captchaChallenge",
        30011: "captchaVersion",
        30012: "captchaActivation",
        30013: "contentRejected",
        30014: "secretsUnavailable",
        30015: "uploadLimit",
        30016: "importBusy",
        30017: "mailNotConfigured",
        30018: "emailCode",
        30019: "emailCodeCooldown",
        30020: "backupBusy",
        30021: "backupMismatch",
        50001: "server",
        50002: "storage",
        50003: "processing",
        50004: "captchaUnavailable",
        50005: "reviewUnavailable",
        50006: "mailUnavailable",
      } as Record<number, string>
    )[error.code];
  if (!key && error.status === 429) key = "rateLimit";
  if (!key && error.status >= 500) key = "server";
  return $t(key ? `common.errors.${key}` : fallbackKey);
}
