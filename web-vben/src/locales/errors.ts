import { $t } from "@vben/locales";
import { ApiError } from "@/api/client";

/** Localize structured error codes, never display arbitrary backend diagnostics. */
export function formatApiError(error: unknown, fallbackKey = "common.errors.unknown"): string {
  if (!(error instanceof ApiError)) return $t(fallbackKey);
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
        50001: "server",
        50002: "storage",
        50003: "processing",
        50004: "captchaUnavailable",
        50005: "reviewUnavailable",
      } as Record<number, string>
    )[error.code];
  if (!key && error.status === 429) key = "rateLimit";
  if (!key && error.status >= 500) key = "server";
  return $t(key ? `common.errors.${key}` : fallbackKey);
}
