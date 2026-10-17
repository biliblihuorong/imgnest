/** 管理端「关于」：当前版本与更新检查（/api/admin/system/version），契约见 docs/openapi.yaml。 */
import { request } from "./client";
import type { components } from "./schema";

export type SystemVersion = components["schemas"]["SystemVersion"];

/** refresh 为 true 时让服务端立刻重新检查（服务端限制为每分钟最多一次）。 */
export function fetchSystemVersion(refresh = false): Promise<SystemVersion> {
  return request<SystemVersion>(`/api/admin/system/version${refresh ? "?refresh=1" : ""}`);
}
