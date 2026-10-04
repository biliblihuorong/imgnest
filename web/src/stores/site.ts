import { defineStore } from "pinia";
import { fetchSite } from "@/api/site";

interface SiteState {
  siteName: string;
  registerEnabled: boolean;
  loaded: boolean;
}

export const useSiteStore = defineStore("site", {
  state: (): SiteState => ({
    siteName: "ImgNest",
    registerEnabled: false,
    loaded: false,
  }),
  actions: {
    /** 确保站点信息已加载；失败保持默认值（ImgNest/false），下次调用重试。 */
    async ensureLoaded(): Promise<void> {
      if (this.loaded) {
        return;
      }
      try {
        const site = await fetchSite();
        this.siteName = site.site_name;
        this.registerEnabled = site.register_enabled;
        this.loaded = true;
      } catch {
        // 拉取失败不抛错，保持默认值
      }
    },
  },
});
