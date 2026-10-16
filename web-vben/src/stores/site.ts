import { defineStore } from "pinia";
import { fetchSite } from "@/api/site";
import type { LoginProvider } from "@/api/types";

interface SiteState {
  siteName: string;
  registerEnabled: boolean;
  galleryEnabled: boolean;
  /** 注册需要邮件验证码。 */
  emailVerification: boolean;
  /** 可以通过邮件找回密码。 */
  passwordReset: boolean;
  /** 服务端扩展提供的额外登录方式（如 SSO），原版为空。 */
  loginProviders: LoginProvider[];
  loaded: boolean;
}

/** 进行中的站点请求：同一页面多处同时触发时只发一次。 */
const pendingRefresh = new WeakMap<object, { token: object; promise: Promise<void> }>();

export const useSiteStore = defineStore("site", {
  state: (): SiteState => ({
    siteName: "ImgNest",
    registerEnabled: false,
    galleryEnabled: false,
    emailVerification: false,
    passwordReset: false,
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
    /** 保存配置后 force 发起新读；仅最新请求可更新状态，其余入口共用在途请求。 */
    refresh({ force = false }: { force?: boolean } = {}): Promise<void> {
      const pending = pendingRefresh.get(this);
      if (pending && !force) return pending.promise;
      const token = {};
      const run = Promise.resolve().then(async () => {
        try {
          const site = await fetchSite();
          if (pendingRefresh.get(this)?.token !== token) return;
          this.siteName = site.site_name;
          this.registerEnabled = site.register_enabled;
          this.galleryEnabled = site.gallery_enabled;
          this.emailVerification = site.email_verification ?? false;
          this.passwordReset = site.password_reset ?? false;
          this.loginProviders = site.login_providers ?? [];
          this.loaded = true;
        } catch {
          // 拉取失败不抛错，保持当前值，下次刷新重试。
        } finally {
          if (pendingRefresh.get(this)?.token === token) pendingRefresh.delete(this);
        }
      });
      pendingRefresh.set(this, { token, promise: run });
      return run;
    },
  },
});
