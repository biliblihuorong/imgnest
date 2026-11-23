import type { RouteLocationRaw, RouteRecordRaw } from "vue-router";
import { getToken } from "@/api/client";
import { landingPath } from "@/app.hooks";
import { useAuthStore } from "@/stores/auth";
import { useSiteStore } from "@/stores/site";
import { settingsRoutes } from "@/views/admin/settings/sections";

/**
 * ImgNest 的路由表（守卫在 router/index.ts，属于 admin-kit 核心）。
 * meta：bare = 不套外壳也不需要登录；guestOnly = 登录后不再显示；public = 未登录也能看；
 * roles = 允许的角色（写在父路由上，子路由继承）。新增页面还要在 app.config.ts 的 menus 里加菜单。
 */

export const ROUTER_NAMES = {
  home: "home",
  login: "login",
  register: "register",
  resetPassword: "reset-password",
  upload: "upload",
  images: "images",
  tokens: "tokens",
  accountSettings: "account-settings",
} as const;

/** admin 子树命名路由；独立于 ROUTER_NAMES，保持既有用户端五键不变。 */
export const ADMIN_ROUTER_NAMES = {
  forbidden: "forbidden",
  adminOverview: "admin-overview",
  adminUsers: "admin-users",
  adminGroups: "admin-groups",
  adminStorages: "admin-storages",
  adminPolicies: "admin-policies",
  adminSettings: "admin-settings",
  adminImages: "admin-images",
  adminAuditLogs: "admin-audit-logs",
  adminLskyImport: "admin-lsky-import",
} as const;

/**
 * 公开首页只在站点开放了游客上传或公开画廊时才有内容；都没开时，未登录的
 * 访客直接去登录页，表现与没有首页时一致。
 */
export async function publicHomeGuard(): Promise<RouteLocationRaw | boolean> {
  if (getToken()) return true;
  const site = useSiteStore();
  await site.ensureLoaded();
  if (!site.guestUpload && !site.galleryEnabled) return { path: "/login" };
  return true;
}

export const routes: RouteRecordRaw[] = [
  // 未登录的访客先看到公开图床首页；已登录（含身份尚未恢复）照旧进控制台。
  {
    path: "/",
    redirect: () => (getToken() ? landingPath(useAuthStore().user?.role) : "/home"),
  },
  {
    path: "/home",
    name: ROUTER_NAMES.home,
    component: () => import("@/views/PublicHomeView.vue"),
    meta: { bare: true, title: "home.title" },
    beforeEnter: publicHomeGuard,
  },
  {
    path: "/login",
    component: () => import("@/shell/AuthLayout.vue"),
    meta: { bare: true, guestOnly: true, title: "common.login" },
    children: [
      { path: "", name: ROUTER_NAMES.login, component: () => import("@/views/LoginView.vue") },
    ],
  },
  {
    path: "/auth/sso",
    component: () => import("@/shell/AuthLayout.vue"),
    meta: { bare: true, title: "common.login" },
    children: [{ path: "", component: () => import("@/views/SsoCallbackView.vue") }],
  },
  {
    path: "/register",
    component: () => import("@/shell/AuthLayout.vue"),
    meta: { bare: true, guestOnly: true, title: "common.register" },
    children: [
      {
        path: "",
        name: ROUTER_NAMES.register,
        component: () => import("@/views/RegisterView.vue"),
      },
    ],
  },
  {
    path: "/reset-password",
    component: () => import("@/shell/AuthLayout.vue"),
    meta: { bare: true, guestOnly: true, title: "common.auth.resetTitle" },
    children: [
      {
        path: "",
        name: ROUTER_NAMES.resetPassword,
        component: () => import("@/views/ResetPasswordView.vue"),
      },
    ],
  },
  {
    path: "/upload",
    name: ROUTER_NAMES.upload,
    component: () => import("@/views/UploadView.vue"),
    meta: { title: "common.nav.upload" },
  },
  {
    path: "/images",
    name: ROUTER_NAMES.images,
    component: () => import("@/views/ImagesView.vue"),
    // 搜索条件写在 query 里；经典布局的标签页按路径区分，否则每搜一次就多一个标签。
    meta: { title: "common.nav.images", fullPathKey: false },
  },
  // 账户只有一个入口：/account 下分「API Token」与「资料与安全」两个标签。
  // activePath 让经典布局的菜单在两个标签上都高亮 /account。
  {
    path: "/account",
    component: () => import("@/views/PassThrough.vue"),
    redirect: { path: "/account/tokens" },
    meta: { title: "account.title" },
    children: [
      {
        path: "tokens",
        name: ROUTER_NAMES.tokens,
        component: () => import("@/views/TokensView.vue"),
        meta: { title: "account.tabs.tokens", activePath: "/account" },
      },
      {
        path: "settings",
        name: ROUTER_NAMES.accountSettings,
        component: () => import("@/views/AccountSettingsView.vue"),
        meta: { title: "account.tabs.profile", activePath: "/account" },
      },
    ],
  },
  // 旧链接（文档、书签、0.1.x 的菜单）。
  { path: "/tokens", redirect: { path: "/account/tokens" } },
  {
    path: "/albums",
    name: "albums",
    component: () => import("@/views/AlbumsView.vue"),
    meta: { title: "common.nav.albums" },
  },
  {
    path: "/albums/:id(\\d+)",
    name: "album-detail",
    component: () => import("@/views/AlbumDetailView.vue"),
    meta: { title: "common.nav.albums", fullPathKey: false },
  },
  {
    path: "/gallery",
    name: "gallery",
    component: () => import("@/views/GalleryView.vue"),
    // 公开画廊：未登录也能看，套同一个外壳。
    meta: { public: true, title: "common.nav.gallery" },
  },
  {
    path: "/dashboard",
    name: "dashboard",
    component: () => import("@/views/DashboardView.vue"),
    meta: { title: "common.nav.dashboard" },
  },
  {
    path: "/forbidden",
    name: ADMIN_ROUTER_NAMES.forbidden,
    component: () => import("@/views/ForbiddenView.vue"),
    meta: { title: "common.forbidden" },
  },
  {
    path: "/admin",
    component: () => import("@/components/admin/AdminLayout.vue"),
    redirect: { path: "/admin/overview" },
    meta: { roles: ["admin"], title: "common.nav.admin" },
    children: [
      {
        path: "overview",
        name: ADMIN_ROUTER_NAMES.adminOverview,
        component: () => import("@/views/admin/OverviewView.vue"),
        meta: { title: "common.nav.overview" },
      },
      {
        path: "users",
        name: ADMIN_ROUTER_NAMES.adminUsers,
        component: () => import("@/views/admin/UsersView.vue"),
        meta: { title: "common.nav.users" },
      },
      {
        path: "groups",
        name: ADMIN_ROUTER_NAMES.adminGroups,
        component: () => import("@/views/admin/GroupsView.vue"),
        meta: { title: "common.nav.groups" },
      },
      {
        path: "storages",
        name: ADMIN_ROUTER_NAMES.adminStorages,
        component: () => import("@/views/admin/StoragesView.vue"),
        meta: { title: "common.nav.storages" },
      },
      {
        path: "policies",
        name: ADMIN_ROUTER_NAMES.adminPolicies,
        component: () => import("@/views/admin/PoliciesView.vue"),
        meta: { title: "common.nav.policies" },
      },
      {
        path: "settings",
        name: ADMIN_ROUTER_NAMES.adminSettings,
        component: () => import("@/views/admin/SettingsView.vue"),
        redirect: { name: "admin-settings-general" },
        meta: { title: "common.nav.settings" },
        children: settingsRoutes(),
      },
      {
        path: "images",
        name: ADMIN_ROUTER_NAMES.adminImages,
        component: () => import("@/views/admin/ImagesAdminView.vue"),
        meta: { title: "common.nav.allImages" },
      },
      {
        path: "audit-logs",
        name: ADMIN_ROUTER_NAMES.adminAuditLogs,
        component: () => import("@/views/admin/AuditLogsView.vue"),
        meta: { title: "common.nav.auditLogs" },
      },
      // 蓝空迁移已并入 站点设置 → 备份/导出；保留旧地址的跳转。
      {
        path: "lsky-import",
        name: ADMIN_ROUTER_NAMES.adminLskyImport,
        redirect: { name: "admin-settings-backup" },
      },
    ],
  },
  { path: "/:pathMatch(.*)*", redirect: "/upload" },
];
