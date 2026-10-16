import { ApiError, getToken, notifyUnauthorized, request } from "./client";
import type { components } from "./schema";

/** 备份目录里的一个 zip 归档；auto 为定时备份写的（只有它们会被自动清理）。 */
export type BackupFile = components["schemas"]["BackupFile"];
export type BackupSchedule = components["schemas"]["BackupSchedule"];
/** location 是服务端配置的 backup.dir。 */
export type BackupOverview = components["schemas"]["BackupOverview"];

const BACKUP_NAME = /^imgnest-\d{8}-\d{6}(-auto)?\.zip$/;

export function getBackups(): Promise<BackupOverview> {
  return request<BackupOverview>("/api/admin/backups");
}

export function createBackup(): Promise<BackupFile> {
  return request<BackupFile>("/api/admin/backups", { method: "POST" });
}

export function deleteBackup(name: string): Promise<{ name: string }> {
  return request<{ name: string }>(`/api/admin/backups/${encodeURIComponent(name)}`, {
    method: "DELETE",
  });
}

export function saveBackupSchedule(schedule: BackupSchedule): Promise<BackupSchedule> {
  return request<BackupSchedule>("/api/admin/backups/schedule", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(schedule),
  });
}

/** 带 Bearer 头下载归档；Token 只进请求头，不进 URL。 */
export async function downloadBackup(name: string): Promise<Blob> {
  if (!BACKUP_NAME.test(name)) throw new ApiError(10001, "", 404);
  const token = getToken();
  const headers = new Headers();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  let response: Response;
  try {
    response = await fetch(`/api/admin/backups/${name}`, { headers, cache: "no-store" });
  } catch {
    throw new ApiError(-1, "", 0);
  }
  if (!response.ok) {
    let code = response.status === 401 ? 20001 : -1;
    let data: unknown;
    try {
      const envelope = (await response.json()) as { code?: unknown; data?: unknown };
      if (typeof envelope.code === "number") code = envelope.code;
      data = envelope.data;
    } catch {
      /* 网关错误页不是 JSON。 */
    }
    notifyUnauthorized(code, token);
    throw new ApiError(code, "", response.status, data);
  }
  return response.blob();
}
