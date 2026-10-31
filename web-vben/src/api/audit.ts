/**
 * 操作日志（/api/admin/audit-logs），契约见 docs/openapi.yaml。
 * 只给管理员看：含账户、IP 和请求 ID，不含请求体。
 */
import { request } from "./client";
import type { components } from "./schema";

export type AuditEntry = components["schemas"]["AuditEntry"];
export type AuditPage = components["schemas"]["AuditPage"];

export interface AuditQuery {
  page?: number;
  size?: number;
  /** 只看某个账户。 */
  userId?: number;
  /** 只看 4xx / 5xx。 */
  failed?: boolean;
}

/** 分页读取操作日志，新的在前。 */
export function listAuditLogs(query: AuditQuery = {}): Promise<AuditPage> {
  const params = new URLSearchParams();
  if (query.page) params.set("page", String(query.page));
  if (query.size) params.set("size", String(query.size));
  if (query.userId) params.set("user_id", String(query.userId));
  if (query.failed) params.set("failed", "true");
  const search = params.toString();
  return request<AuditPage>(`/api/admin/audit-logs${search ? `?${search}` : ""}`);
}
