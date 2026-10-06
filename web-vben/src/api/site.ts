import { request } from "./client";
import type { SiteInfo } from "./types";

/** 公开站点信息：站点名与注册开关，不含其他 settings。 */
export function fetchSite(): Promise<SiteInfo> {
  return request<SiteInfo>("/api/site");
}
