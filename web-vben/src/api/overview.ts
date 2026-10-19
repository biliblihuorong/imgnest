/** 管理端「概览」：站点级用量与上传统计（/api/admin/overview），契约见 docs/openapi.yaml。 */
import { request } from "./client";
import type { components } from "./schema";

export type AdminOverview = components["schemas"]["AdminOverview"];
export type AdminStorageUsage = components["schemas"]["AdminStorageUsage"];
export type AdminPolicyUsage = components["schemas"]["AdminPolicyUsage"];
export type AdminUploadDay = components["schemas"]["AdminUploadDay"];
export type AdminUserUsage = components["schemas"]["AdminUserUsage"];

export function fetchAdminOverview(): Promise<AdminOverview> {
  return request<AdminOverview>("/api/admin/overview");
}
