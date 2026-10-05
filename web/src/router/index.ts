import {
  createRouter,
  createWebHistory,
  type RouteLocationNormalized,
  type RouteLocationRaw,
  type Router,
} from "vue-router";
import { TOKEN_STORAGE_KEY } from "@/api/client";
import { useAuthStore } from "@/stores/auth";

declare module "vue-router" {
  interface RouteMeta {
    /** 裸布局（登录/注册页，不渲染站点头部）。 */
    bare?: boolean;
    /** 管理后台子树（要求 role=admin，由 adminGuard 把关）。 */
    admin?: boolean;
  }
}

export const ROUTER_NAMES = {
  login: "login",
  register: "register",
  upload: "upload",
  images: "images",
  tokens: "tokens",
} as const;

/** admin 子树命名路由；独立于 ROUTER_NAMES，保持既有用户端五键不变。 */
export const ADMIN_ROUTER_NAMES = {
  forbidden: "forbidden",
  adminUsers: "admin-users",
  adminGroups: "admin-groups",
  adminStorages: "admin-storages",
  adminPolicies: "admin-policies",
  adminSettings: "admin-settings",
  adminImages: "admin-images",
} as const;

const PUBLIC_PATHS = new Set<string>(["/login", "/register", "/gallery"]);

/**
 * admin 子树守卫（挂在 /admin 父路由的 beforeEnter 上，进入子树即触发）：
 * - 未登录（无 token）→ /login?redirect=<fullPath>；
 * - 有 token 但 user 尚未加载（刷新后 restore() 未完成）→ 先 await restore()
 *   拿到真实身份再判定；restore 失败会清空登录态，按未登录处理；
 * - 已登录但 user.role !== "admin" → 重定向 /forbidden（403 页），不进 admin 页面。
 */
export async function adminGuard(to: RouteLocationNormalized): Promise<RouteLocationRaw | boolean> {
  const auth = useAuthStore();
  if (!auth.user) {
    if (!auth.token) {
      return { path: "/login", query: { redirect: to.fullPath } };
    }
    await auth.restore();
    if (!auth.user) {
      return { path: "/login", query: { redirect: to.fullPath } };
    }
  }
  if (auth.user.role !== "admin") {
    return { path: "/forbidden" };
  }
  return true;
}

/** 创建带全局守卫的路由实例；守卫只做本地 token 存在性检查，真实校验靠 API 401。 */
export function setupRouter(): Router {
  const router = createRouter({
    history: createWebHistory(),
    routes: [
      { path: "/", redirect: "/upload" },
      {
        path: "/login",
        name: ROUTER_NAMES.login,
        component: () => import("@/views/LoginView.vue"),
        meta: { bare: true },
      },
      {
        path: "/register",
        name: ROUTER_NAMES.register,
        component: () => import("@/views/RegisterView.vue"),
        meta: { bare: true },
      },
      {
        path: "/upload",
        name: ROUTER_NAMES.upload,
        component: () => import("@/views/UploadView.vue"),
      },
      {
        path: "/images",
        name: ROUTER_NAMES.images,
        component: () => import("@/views/ImagesView.vue"),
      },
      {
        path: "/tokens",
        name: ROUTER_NAMES.tokens,
        component: () => import("@/views/TokensView.vue"),
      },
      {
        path: "/albums",
        name: "albums",
        component: () => import("@/views/AlbumsView.vue"),
      },
      {
        path: "/gallery",
        name: "gallery",
        component: () => import("@/views/GalleryView.vue"),
      },
      {
        path: "/forbidden",
        name: ADMIN_ROUTER_NAMES.forbidden,
        component: () => import("@/views/ForbiddenView.vue"),
      },
      {
        path: "/admin",
        component: () => import("@/components/admin/AdminLayout.vue"),
        redirect: { path: "/admin/users" },
        meta: { admin: true },
        beforeEnter: adminGuard,
        children: [
          {
            path: "users",
            name: ADMIN_ROUTER_NAMES.adminUsers,
            component: () => import("@/views/admin/UsersView.vue"),
          },
          {
            path: "groups",
            name: ADMIN_ROUTER_NAMES.adminGroups,
            component: () => import("@/views/admin/GroupsView.vue"),
          },
          {
            path: "storages",
            name: ADMIN_ROUTER_NAMES.adminStorages,
            component: () => import("@/views/admin/StoragesView.vue"),
          },
          {
            path: "policies",
            name: ADMIN_ROUTER_NAMES.adminPolicies,
            component: () => import("@/views/admin/PoliciesView.vue"),
          },
          {
            path: "settings",
            name: ADMIN_ROUTER_NAMES.adminSettings,
            component: () => import("@/views/admin/SettingsView.vue"),
          },
          {
            path: "images",
            name: ADMIN_ROUTER_NAMES.adminImages,
            component: () => import("@/views/admin/ImagesAdminView.vue"),
          },
        ],
      },
      { path: "/:pathMatch(.*)*", redirect: "/upload" },
    ],
  });

  router.beforeEach((to) => {
    let token: string | null = null;
    try {
      token = localStorage.getItem(TOKEN_STORAGE_KEY);
    } catch {
      token = null;
    }
    const isPublic = PUBLIC_PATHS.has(to.path);
    if (!isPublic && !token) {
      return { path: "/login", query: { redirect: to.fullPath } };
    }
    if (isPublic && token) {
      return { path: "/upload" };
    }
    return true;
  });

  return router;
}
