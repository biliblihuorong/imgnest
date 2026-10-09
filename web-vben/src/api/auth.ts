import { request } from "./client";
import type { LoginData, UserView } from "./types";

export interface RegisterPayload {
  captcha_token?: string;
  username: string;
  email: string;
  password: string;
}

/** 登录并颁发 web token（有效期 24 小时）；字段契约见 openapi /api/auth/login。 */
export function login(
  email: string,
  password: string,
  captchaToken?: string,
  signal?: AbortSignal,
): Promise<LoginData> {
  return request<LoginData>("/api/auth/login", {
    method: "POST",
    signal,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      email,
      password,
      ...(captchaToken === undefined ? {} : { captcha_token: captchaToken }),
    }),
  });
}

/** 注册普通用户；注册响应不颁发 token。 */
export function register(payload: RegisterPayload, signal?: AbortSignal): Promise<UserView> {
  return request<UserView>("/api/auth/register", {
    method: "POST",
    signal,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

/** 当前登录用户信息。 */
export function me(): Promise<UserView> {
  return request<UserView>("/api/auth/me");
}

/** 吊销当前 bearer token；失败可忽略（本地登出不受影响）。 */
export function logout(): Promise<null> {
  return request<null>("/api/auth/logout", { method: "POST" });
}

/** 修改密码；成功后服务端吊销该用户全部 token，需要重新登录。 */
export function changePassword(currentPassword: string, newPassword: string): Promise<null> {
  return request<null>("/api/auth/password", {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
  });
}

/** 保存显示名称；服务端 trim，空串表示清除并回退用户名。返回刷新后的用户视图。 */
export function updateDisplayName(displayName: string): Promise<UserView> {
  return request<UserView>("/api/auth/profile", {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ display_name: displayName }),
  });
}

/** 用单点登录回调给出的一次性票据换取 web token；响应与 /api/auth/login 相同。 */
export function exchangeSsoTicket(ticket: string, signal?: AbortSignal): Promise<LoginData> {
  return request<LoginData>("/api/auth/sso/exchange", {
    method: "POST",
    signal,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ticket }),
  });
}
