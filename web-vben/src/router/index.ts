import {
  createRouter,
  createWebHistory,
  type RouteLocationNormalized,
  type RouteLocationRaw,
  type Router,
  type RouterHistory,
} from "vue-router";
import type { CurrentUser } from "@/api/types";
import { appConfig } from "@/app.config";
import { canAccess, guestRedirect, homePath, type AccessRule } from "@/app.hooks";
import { useAuthStore } from "@/stores/auth";
import { routes } from "./routes";

declare module "vue-router" {
  /** 访问要求字段（roles 等）来自 app.hooks.ts 的 AccessRule，项目在那里扩展。 */
  interface RouteMeta extends AccessRule {
    /** i18n key：浏览器标题、经典布局的标签页和面包屑都用它。 */
    title?: string;
    /** 裸页面：不套后台外壳（登录页、公开首页、单点登录回调等），也不需要登录。 */
    bare?: boolean;
    /** 只给未登录的人看（登录、注册、找回密码）：已登录时直接去 app.hooks.ts 的 homePath。 */
    guestOnly?: boolean;
    /** 公开页面：套后台外壳，但未登录也能访问（例如公开画廊）；已登录时照常检查访问要求。 */
    public?: boolean;
  }
}

const isBare = (to: RouteLocationNormalized) => to.matched.some((record) => record.meta.bare);
const isPublic = (to: RouteLocationNormalized) => to.matched.some((record) => record.meta.public);
const isGuestOnly = (to: RouteLocationNormalized) =>
  to.path === appConfig.loginPath || to.matched.some((record) => record.meta.guestOnly);

/** 路径上每一层的访问要求都必须满足（父路由的限制自动作用于子路由）。 */
export function allowedFor(to: RouteLocationNormalized, user: CurrentUser): boolean {
  return to.matched.every((record) => canAccess(user, record.meta));
}

/** 不需要登录就能看的页面（裸页面或公开页面）。退出登录时停在这些页面上不用离开。 */
export function isOpenRoute(to: RouteLocationNormalized): boolean {
  return to.matched.length > 0 && (isBare(to) || isPublic(to));
}

/**
 * 只接受站内、已知、需要登录且不是登录页的跳转目标，防止开放重定向。
 * 不合格时去 fallback：登录页里传刚登录用户的 homePath，不传就是 appConfig.homePath。
 */
export function safeRedirect(
  value: unknown,
  router: Router,
  fallback: string = appConfig.homePath,
): string {
  if (typeof value !== "string" || !value.startsWith("/") || value.startsWith("//"))
    return fallback;
  if (/[\\\u0000-\u001f]/.test(value)) return fallback;
  const resolved = router.resolve(value);
  if (
    resolved.matched.length === 0 ||
    resolved.name === "not-found" ||
    resolved.matched.some((record) => record.path.includes(":pathMatch")) ||
    resolved.matched.some((record) => record.meta.bare || record.meta.guestOnly)
  )
    return fallback;
  return resolved.fullPath;
}

export async function authGuard(to: RouteLocationNormalized): Promise<RouteLocationRaw | boolean> {
  const auth = useAuthStore();
  if (isGuestOnly(to)) {
    // 已登录再访问登录/注册页时直接进后台。
    if (auth.token) {
      await auth.restore();
      if (auth.user) return { path: homePath(auth.user) };
    }
    return true;
  }
  if (isBare(to)) return true;
  if (!auth.token) {
    if (isPublic(to)) return true;
    const elsewhere = await guestRedirect(to);
    if (elsewhere !== undefined) return elsewhere;
    return { path: appConfig.loginPath, query: { redirect: to.fullPath } };
  }
  await auth.restore();
  if (!auth.user) {
    if (isPublic(to)) return true;
    return { path: appConfig.loginPath, query: { redirect: to.fullPath } };
  }
  if (!allowedFor(to, auth.user)) return { path: "/forbidden" };
  return true;
}

export function setupRouter(history: RouterHistory = createWebHistory()): Router {
  const router = createRouter({ history, routes });
  router.beforeEach(authGuard);
  return router;
}
