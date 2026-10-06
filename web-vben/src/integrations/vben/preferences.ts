import { defineOverridesPreferences, updatePreferences } from "@vben/preferences";
import logo from "@/assets/imgnest-mark.svg";
import avatar from "@/assets/account-avatar.svg";

/**
 * 不显示版权栏，也不在经典布局的偏好抽屉里提供版权设置；
 * 同时清掉 Vben 模板自带的公司名、链接与备案号默认值。
 */
export const IMGNEST_COPYRIGHT = {
  enable: false,
  settingShow: false,
  companyName: "",
  companySiteLink: "",
  date: "",
  icp: "",
  icpLink: "",
} as const;

/** 浏览器里已缓存的旧偏好会盖过默认覆盖项，启动时再强制写一次。 */
export function enforceImgnestPreferences(): void {
  updatePreferences({ copyright: { ...IMGNEST_COPYRIGHT } });
}

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
  copyright: { ...IMGNEST_COPYRIGHT },
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
