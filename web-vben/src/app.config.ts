import type { Component } from "vue";
import {
  ImagePlus,
  Inbox,
  InspectionPanel,
  LayoutGrid,
  List,
  LockKeyhole,
  Settings,
  UserRoundPen,
} from "@vben/icons";
// logo 用于侧栏、登录页和浏览器图标；loadingLogo 是同一图标的动画版，只在启动加载画面显示。
// build/app-config-html.ts 会读这两行，改名或改路径时保持 `import xxx from "…"` 的写法。
import logo from "@/assets/imgnest-mark.svg";
import loadingLogo from "@/assets/imgnest-loading.svg";
import avatar from "@/assets/account-avatar.svg";
import type { AccessRule } from "@/app.hooks";

/**
 * ImgNest 的静态配置：品牌、本地存储前缀、菜单。行为扩展（动态菜单、落地页、公开首页、头像、
 * 站点名、命令面板）在 app.hooks.ts。外壳（src/shell，来自 admin-kit）只读这两个文件。
 */

export type Role = "admin" | "user";

/** 访问要求（roles 以及 app.hooks.ts 里 AccessRule 加的字段）只控制显示，真正的拦截靠路由 meta，两边要写一致。 */
export interface MenuItem extends AccessRule {
  /** i18n key，例如 "common.nav.dashboard"。 */
  name: string;
  path: string;
  icon?: Component;
  /** 菜单小红点。 */
  dot?: boolean;
  /** 未登录也显示（对应的路由 meta 也要写 public: true）。 */
  public?: boolean;
  /** 有子项时：Marvis 侧栏展开成带标题的分组，经典布局是可折叠的父菜单。 */
  children?: MenuItem[];
}

export const appConfig = {
  /** 站点名的默认值；运行时以站点设置里的站点名为准（app.hooks.ts 的 loadSite）。 */
  name: "ImgNest",
  /** Marvis 侧栏站名旁的小徽标（i18n key）。 */
  badge: "workspace.brandBadge",
  logo,
  /** 启动加载画面用的动画图标（照片落进巢里）。 */
  loadingLogo,
  defaultAvatar: avatar,
  /** localStorage 前缀：token 存在 imgnest.token，偏好在 imgnest-vben-v1-*，不要改，否则用户要重新登录。 */
  storagePrefix: "imgnest",
  defaultShell: "marvis" as "marvis" | "classic",
  /** 普通用户的落地页；管理员见 app.hooks.ts 的 landingPath。 */
  homePath: "/upload",
  loginPath: "/login",
  /** 账户页（API Token、资料与安全）。 */
  accountPath: "/account" as string,
} as const;

/** 公开画廊；只有站点开启了画廊才出现在菜单里（app.hooks.ts 的 resolveMenus）。 */
export const galleryMenu: MenuItem = {
  name: "common.nav.gallery",
  path: "/gallery",
  icon: LayoutGrid,
  public: true,
};

/** 菜单树。新增页面：在 router/routes.ts 加路由，再在这里加一项。 */
export const menus: MenuItem[] = [
  { name: "common.nav.upload", path: "/upload", icon: ImagePlus },
  { name: "common.nav.images", path: "/images", icon: LayoutGrid },
  { name: "common.nav.albums", path: "/albums", icon: Inbox },
  { name: "common.nav.dashboard", path: "/dashboard", icon: InspectionPanel },
  { name: "common.nav.tokens", path: "/account", icon: LockKeyhole },
  galleryMenu,
  {
    name: "common.nav.admin",
    path: "/admin",
    icon: Settings,
    roles: ["admin"],
    children: [
      { name: "common.nav.overview", path: "/admin/overview", icon: InspectionPanel },
      { name: "common.nav.users", path: "/admin/users", icon: UserRoundPen },
      { name: "common.nav.groups", path: "/admin/groups", icon: InspectionPanel },
      { name: "common.nav.storages", path: "/admin/storages", icon: Inbox },
      { name: "common.nav.policies", path: "/admin/policies", icon: List },
      { name: "common.nav.settings", path: "/admin/settings", icon: Settings },
      { name: "common.nav.allImages", path: "/admin/images", icon: LayoutGrid },
      { name: "common.nav.auditLogs", path: "/admin/audit-logs", icon: List },
    ],
  },
];
