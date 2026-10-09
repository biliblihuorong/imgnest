import { defineStore } from "pinia";
import { fetchSite } from "@/api/site";
import type { LoginProvider } from "@/api/types";

interface SiteState {
  siteName: string;
  registerEnabled: boolean;
  galleryEnabled: boolean;
  /** 服务端扩展提供的额外登录方式（如 SSO），原版为空。 */
  loginProviders: LoginProvider[];
  loaded: boolean;
}

export const useSiteStore = defineStore("site", {
  state: (): SiteState => ({
    siteName: "ImgNest",
    registerEnabled: false,
    galleryEnabled: false,
    loginProviders: [],
    loaded: false,
  }),
  actions: {
    /** 确保站点信息已加载；失败保持默认值（ImgNest/false），下次调用重试。 */
    async ensureLoaded(): Promise<void> {
      if (this.loaded) {
        return;
      }
      await this.refresh();
    },
    /** 无论是否加载过都重新拉取站点信息，供登录/注册页等低频入口保持最新开关。 */
    async refresh(): Promise<void> {
      try {
        const site = await fetchSite();
        this.siteName = site.site_name;
        this.registerEnabled = site.register_enabled;
        this.galleryEnabled = site.gallery_enabled;
        this.loginProviders = site.login_providers ?? [];
        this.loaded = true;
      } catch {
        // 拉取失败不抛错，保持默认值
      }
    },
  },
});
