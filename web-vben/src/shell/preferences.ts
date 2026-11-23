import { defineOverridesPreferences, updatePreferences } from "@vben/preferences";
import { appConfig } from "@/app.config";

/** 不显示 Vben 模板自带的版权栏、公司名和备案号。 */
export const NO_COPYRIGHT = {
  enable: false,
  settingShow: false,
  companyName: "",
  companySiteLink: "",
  date: "",
  icp: "",
  icpLink: "",
} as const;

/** 浏览器里已缓存的旧偏好会盖过默认覆盖项，启动时再强制写一次。 */
export function enforcePreferences(): void {
  updatePreferences({ app: { enablePreferences: false }, copyright: { ...NO_COPYRIGHT } });
}

/** Vben 偏好覆盖：关闭没有后端支持的演示能力（刷新 Token、锁屏、通知等）。 */
export const appPreferences = defineOverridesPreferences({
  app: {
    name: appConfig.name,
    defaultHomePath: appConfig.homePath,
    defaultAvatar: appConfig.defaultAvatar,
    accessMode: "frontend",
    locale: "zh-CN",
    authPageLayout: "panel-center",
    enableRefreshToken: false,
    enableCheckUpdates: false,
    enableCopyPreferences: false,
    loginExpiredMode: "page",
    // 两套布局共用 MarvisSettingsModal；不显示 Vben 自带的偏好抽屉。
    enablePreferences: false,
  },
  copyright: { ...NO_COPYRIGHT },
  logo: { source: appConfig.logo, sourceDark: appConfig.logo },
  theme: { mode: "auto" },
  // 管理页常有未提交表单和敏感数据，不缓存页面、不持久化标签页。
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
