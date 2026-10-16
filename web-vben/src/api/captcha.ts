import { ApiError, request } from "./client";

export type CaptchaAction = "login" | "register" | "reset";
export interface PublicCaptchaConfig {
  enabled: boolean;
  provider: "turnstile" | "";
  site_key: string;
  version: number;
}
export interface CaptchaCandidate {
  provider: "turnstile";
  site_key: string;
  hostnames: string[];
  secret_configured: boolean;
  version: number;
}
export interface CaptchaDraft extends CaptchaCandidate {
  tested: boolean;
  tested_actions: CaptchaAction[];
}
export interface CaptchaAdminView {
  version: number;
  enabled: boolean;
  active: CaptchaCandidate | null;
  draft: CaptchaDraft | null;
  configuration_available: boolean;
}
/** One-step configuration: enabling needs a fresh token rendered with the submitted site key. */
export interface CaptchaConfigurePayload {
  expected_version: number;
  enabled: boolean;
  site_key?: string;
  hostnames?: string[];
  secret?: string;
  captcha_token?: string;
}

/** An unreadable response cannot authorize password-only authentication. */
export async function fetchCaptcha(signal?: AbortSignal): Promise<PublicCaptchaConfig> {
  const data = await request<PublicCaptchaConfig>("/api/auth/captcha", { signal });
  if (
    !data ||
    typeof data.enabled !== "boolean" ||
    !Number.isSafeInteger(data.version) ||
    data.version < 0 ||
    typeof data.site_key !== "string" ||
    !["", "turnstile"].includes(data.provider) ||
    (data.enabled && (data.provider !== "turnstile" || !data.site_key.trim()))
  ) {
    throw new ApiError(50004, "CAPTCHA configuration unavailable", 503);
  }
  return data;
}
export function getCaptchaSettings(signal?: AbortSignal): Promise<CaptchaAdminView> {
  return request<CaptchaAdminView>("/api/admin/captcha", { signal });
}
export function configureCaptcha(
  payload: CaptchaConfigurePayload,
  signal?: AbortSignal,
): Promise<CaptchaAdminView> {
  return request<CaptchaAdminView>("/api/admin/captcha", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
    signal,
  });
}
