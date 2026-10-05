import { ApiError, request } from "./client";

export type CaptchaAction = "login" | "register";
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
export interface CaptchaDraftPayload {
  expected_version: number;
  provider: "turnstile";
  site_key: string;
  hostnames: string[];
  secret?: string;
  clear_secret?: boolean;
}
export interface CaptchaActivationPayload {
  expected_version: number;
  enabled: boolean;
  acknowledge_legacy_incompatibility: boolean;
  acknowledge_v1_unprotected: boolean;
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
function mutate(
  path: string,
  method: "PUT" | "POST",
  payload: object,
  signal?: AbortSignal,
): Promise<CaptchaAdminView> {
  return request<CaptchaAdminView>(path, {
    method,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
    signal,
  });
}
export function saveCaptchaDraft(
  payload: CaptchaDraftPayload,
  signal?: AbortSignal,
): Promise<CaptchaAdminView> {
  return mutate("/api/admin/captcha/draft", "PUT", payload, signal);
}
export function testCaptchaDraft(
  payload: { expected_version: number; captcha_token: string; action: CaptchaAction },
  signal?: AbortSignal,
): Promise<CaptchaAdminView> {
  return mutate("/api/admin/captcha/test", "POST", payload, signal);
}
export function setCaptchaActivation(
  payload: CaptchaActivationPayload,
  signal?: AbortSignal,
): Promise<CaptchaAdminView> {
  return mutate("/api/admin/captcha/activation", "POST", payload, signal);
}
