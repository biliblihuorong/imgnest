import { createRouter, createWebHistory, type Router } from "vue-router";
import { TOKEN_STORAGE_KEY } from "@/api/client";

declare module "vue-router" {
  interface RouteMeta {
    /** 裸布局（登录/注册页，不渲染站点头部）。 */
    bare?: boolean;
  }
}

export const ROUTER_NAMES = {
  login: "login",
  register: "register",
  upload: "upload",
  images: "images",
  tokens: "tokens",
} as const;

const PUBLIC_PATHS = new Set<string>(["/login", "/register"]);

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
