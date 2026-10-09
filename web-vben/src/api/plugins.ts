/**
 * 扩展插件设置（/api/admin/plugins）。社区版没有插件，列表为空。
 * 类型派生自 docs/openapi.yaml。
 */
import { request } from "./client";
import type { components } from "./schema";

export type PluginSettings = components["schemas"]["PluginSettings"];
export type PluginSettingField = components["schemas"]["PluginSettingField"];
export type PluginSettingOption = components["schemas"]["PluginSettingOption"];
export type PluginSettingStatus = components["schemas"]["PluginSettingStatus"];

/** 已保存的密钥在读取时以此占位；原样提交表示「不修改」。 */
export const SECRET_KEPT = "__imgnest_secret_kept__";

/** 列表项的稳定标识，由服务端分配，用来在保存时保留该项的密钥。 */
export const ITEM_KEY = "_key";

export function listPluginSettings(): Promise<PluginSettings[]> {
  return request<PluginSettings[]>("/api/admin/plugins");
}

export function savePluginSettings(
  name: string,
  values: Record<string, unknown>,
): Promise<PluginSettings> {
  return request<PluginSettings>(`/api/admin/plugins/${encodeURIComponent(name)}/settings`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ values }),
  });
}
