import { defineOverridesPreferences } from "@vben/preferences";
import logo from "@/assets/imgnest-mark.svg";
import avatar from "@/assets/account-avatar.svg";

/** 真正的 Vben 偏好入口；只关闭没有 ImageNest 业务支持的演示能力。 */
export const imgnestPreferences = defineOverridesPreferences({
  app: {
    name: "ImgNest",
    defaultHomePath: "/upload",
    defaultAvatar: avatar,
    accessMode: "frontend",
    locale: "zh-CN",
    authPageLayout: "panel-center",
    enableRefreshToken: false,
    enableCheckUpdates: false,
    enableCopyPreferences: false,
    loginExpiredMode: "page",
  },
  copyright: { enable: false },
  logo: { source: logo, sourceDark: logo },
  theme: { mode: "auto" },
  // 不缓存含 Token 明文、EXIF 或未提交表单的业务页面。
  tabbar: { keepAlive: false, persist: false, visitHistory: false },
  transition: { enable: false, loading: false, progress: false },
  widget: {
    languageToggle: true,
    timezone: false,
    notification: false,
    lockScreen: false,
    logoutButtonPosition: "user-dropdown",
  },
  shortcutKeys: { globalLockScreen: false },
});
