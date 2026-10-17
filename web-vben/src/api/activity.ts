/**
 * 账户活动（/api/account/activity、/api/admin/users/{id}/activity），契约见 docs/openapi.yaml。
 * 只给账户本人和管理员看：含登录 IP 与浏览器信息。
 */
import { request } from "./client";
import type { components } from "./schema";

export type ActivityView = components["schemas"]["Activity"];
export type LoginRecord = components["schemas"]["LoginRecord"];
export type UsageDay = components["schemas"]["UsageDay"];

/** 当前账户的活动。 */
export function fetchMyActivity(): Promise<ActivityView> {
  return request<ActivityView>("/api/account/activity");
}

/** 管理员查看任一账户的活动。 */
export function fetchUserActivity(userId: number): Promise<ActivityView> {
  return request<ActivityView>(`/api/admin/users/${userId}/activity`);
}
