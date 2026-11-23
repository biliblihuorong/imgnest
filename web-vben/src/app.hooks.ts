import { defineAsyncComponent, markRaw, type Ref } from "vue";
import type { RouteLocationNormalized, RouteLocationRaw } from "vue-router";
import { ApiError } from "@/api/client";
import type { CurrentUser } from "@/api/types";
import { appConfig, galleryMenu, menus, type MenuItem, type Role } from "@/app.config";
import { useUserAvatar } from "@/components/account/useUserAvatar";
import { formatApiError } from "@/locales/errors";
import type { ShellExtensions } from "@/shell/extensions";
import type { SiteBrand } from "@/shell/site";
import { useSiteStore } from "@/stores/site";
import { useSystemStore } from "@/stores/system";

/**
 * ImgNest 接入 admin-kit 外壳的扩展点。外壳（kit.manifest.json 里列出的核心文件）只调用这里，
 * 不认识图床业务。核心文件不要在本仓库里改，改动提回 admin-kit 再用 kit:sync 同步过来。
 */

export interface AccessRule {
  /** 允许的角色；不填表示所有已登录用户。 */
  roles?: Role[];
}

export function canAccess(user: CurrentUser, rule: AccessRule): boolean {
  return !rule.roles || rule.roles.includes(user.role);
}

/** 管理员进仪表盘，普通用户进上传页；身份还没恢复时按普通用户处理。 */
export function landingPath(role?: string): string {
  return role === "admin" ? "/dashboard" : appConfig.homePath;
}

/** 未登录时点 Logo 去公开画廊（画廊没开时路由守卫会再送去首页或登录页）。 */
export function homePath(user: CurrentUser | null): string {
  return user ? landingPath(user.role) : "/gallery";
}

/**
 * 菜单随站点设置变化：公开画廊开了才显示画廊；有新版本时管理员的「站点管理」和其中的
 * 「站点设置」带小红点。外壳在响应式上下文里调用，store 一变菜单就刷新。
 */
export const resolveMenus: (user: CurrentUser | null) => MenuItem[] = () => {
  const site = useSiteStore();
  const system = useSystemStore();
  const updateAvailable = system.updateAvailable;
  return menus
    .filter((menu) => menu !== galleryMenu || site.galleryEnabled)
    .map((menu) =>
      menu.path === "/admin" && updateAvailable
        ? {
            ...menu,
            dot: true,
            children: menu.children?.map((child) =>
              child.path === "/admin/settings" ? { ...child, dot: true } : child,
            ),
          }
        : menu,
    );
};

/** 站点开放了公开画廊或游客上传时，未登录访客去公开首页，而不是登录页。 */
export const guestRedirect: (
  to: RouteLocationNormalized,
) => Promise<RouteLocationRaw | undefined> = async () => {
  const site = useSiteStore();
  await site.ensureLoaded();
  return site.guestUpload || site.galleryEnabled ? { path: "/home" } : undefined;
};

/** 头像先显示本地默认图，远程头像（WeAvatar/Gravatar）加载成功后再替换。 */
export function useAvatar(user: Readonly<Ref<CurrentUser | null>>): Readonly<Ref<string>> {
  return useUserAvatar(user);
}

/** 只有管理员需要版本信息（驱动菜单小红点）；换成非管理员时清掉上一位管理员的提醒。 */
export function onIdentityChange(user: CurrentUser | null): void {
  const system = useSystemStore();
  if (user?.role === "admin") void system.load();
  else if (system.version) system.clear();
}

/** 站点名来自站点设置；之后管理员改了设置，site store 刷新时会自己调用 applySite。 */
export async function loadSite(): Promise<Partial<SiteBrand> | undefined> {
  const site = useSiteStore();
  await site.ensureLoaded();
  return { name: site.siteName };
}

/** 接口错误统一走 locales/errors.ts（按错误码和原因代码翻译，不显示后端诊断信息）。 */
export const describeError = (error: unknown): string | undefined =>
  error instanceof ApiError ? formatApiError(error) : undefined;

export const shellExtensions: ShellExtensions = {
  // Ctrl/⌘+K 是图片检索（带搜索条件补全），页面跳转放在结果下方。
  commandPalette: markRaw(
    defineAsyncComponent(() => import("@/components/search/ImageCommandPalette.vue")),
  ),
  searchLabel: "workspace.search",
  sidebarItemExtra: markRaw(
    defineAsyncComponent(() => import("@/components/albums/SidebarAlbumsExtra.vue")),
  ),
  userMenu: (user) =>
    user.role === "admin" ? [{ label: "common.nav.admin", path: "/admin/overview" }] : [],
};
