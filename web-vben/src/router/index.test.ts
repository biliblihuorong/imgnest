import { createPinia, setActivePinia } from "pinia";
import { useAuthStore } from "@/stores/auth";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { TOKEN_STORAGE_KEY } from "@/api/client";
import { ROUTER_NAMES, setupRouter } from "./index";

// 守卫测试只关心导航行为：用轻量 stub 替换真实视图模块，避免在测试阶段加载 naive-ui
vi.mock("@/components/layout/AuthLayout.vue", () => ({
  default: { name: "AuthLayoutStub", template: "<RouterView />" },
}));
vi.mock("@/views/LoginView.vue", () => ({
  default: { name: "LoginViewStub", template: "<div />" },
}));
vi.mock("@/views/RegisterView.vue", () => ({
  default: { name: "RegisterViewStub", template: "<div />" },
}));
vi.mock("@/views/UploadView.vue", () => ({
  default: { name: "UploadViewStub", template: "<div />" },
}));
vi.mock("@/views/ImagesView.vue", () => ({
  default: { name: "ImagesViewStub", template: "<div />" },
}));
vi.mock("@/views/TokensView.vue", () => ({
  default: { name: "TokensViewStub", template: "<div />" },
}));

vi.mock("@/views/GalleryView.vue", () => ({ default: { template: "<div />" } }));
vi.mock("@/views/AlbumsView.vue", () => ({ default: { template: "<div />" } }));
vi.mock("@/views/DashboardView.vue", () => ({ default: { template: "<div />" } }));

describe("router 守卫", () => {
  beforeEach(() => {
    localStorage.clear();
    setActivePinia(createPinia());
  });

  it("未登录访问受保护路由时跳转 /login 并携带 redirect", async () => {
    const router = setupRouter();
    await router.push("/images");
    await router.isReady();

    expect(router.currentRoute.value.path).toBe("/login");
    expect(router.currentRoute.value.query.redirect).toBe("/images");
  });

  it("未登录访问 /upload 同样跳转登录并记录 redirect", async () => {
    const router = setupRouter();
    await router.push("/upload");
    await router.isReady();

    expect(router.currentRoute.value.path).toBe("/login");
    expect(router.currentRoute.value.query.redirect).toBe("/upload");
  });

  it("未登录访问公开路由 /login 直接放行", async () => {
    const router = setupRouter();
    await router.push("/login");
    await router.isReady();

    expect(router.currentRoute.value.path).toBe("/login");
    expect(router.currentRoute.value.query.redirect).toBeUndefined();
  });

  it("已登录访问 /login 时跳转 /upload", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|token");
    const auth = useAuthStore();
    auth.user = { id: 1, role: "user" } as never;
    const router = setupRouter();
    await router.push("/login");
    await router.isReady();

    expect(router.currentRoute.value.path).toBe("/upload");
  });

  it("已登录访问受保护路由时放行", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|token");
    useAuthStore().user = { id: 1, role: "user" } as never;
    const router = setupRouter();
    await router.push("/upload");
    await router.isReady();

    expect(router.currentRoute.value.path).toBe("/upload");
  });
  it("storage 写失败后的内存会话仍可访问受保护路由", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          code: 0,
          data: {
            token: "7|memory",
            user: { id: 1, role: "user" },
            expires_at: "2026-10-14T00:00:00Z",
          },
        }),
      ),
    );
    vi.stubGlobal("fetch", fetchMock);
    const setItem = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("quota", "QuotaExceededError");
    });
    try {
      await useAuthStore().login("alice@example.com", "password-123");
      const router = setupRouter();
      await router.push("/images");
      expect(router.currentRoute.value.path).toBe("/images");
    } finally {
      setItem.mockRestore();
      vi.unstubAllGlobals();
    }
  });

  it("身份尚未恢复时守卫先等待 restore 再放行", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|token");
    const user = { id: 1, role: "user" };
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ code: 0, message: "ok", data: user }), {
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );
    try {
      const router = setupRouter();
      await router.push("/upload");
      await router.isReady();

      expect(router.currentRoute.value.path).toBe("/upload");
      expect(useAuthStore().user).toEqual(user);
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("恢复身份失败时跳转登录", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|token");
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ code: 20001, message: "expired", data: null }), {
          status: 401,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );
    try {
      const router = setupRouter();
      await router.push("/images");
      await router.isReady();

      expect(router.currentRoute.value.path).toBe("/login");
      expect(router.currentRoute.value.query.redirect).toBe("/images");
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("未知路径重定向到 /upload 并受守卫约束", async () => {
    const router = setupRouter();
    await router.push("/no-such-page");
    await router.isReady();

    expect(router.currentRoute.value.path).toBe("/login");
    expect(router.currentRoute.value.query.redirect).toBe("/upload");
  });

  it("ROUTER_NAMES 暴露七个命名路由", () => {
    expect(Object.keys(ROUTER_NAMES).sort()).toEqual([
      "accountSettings",
      "images",
      "login",
      "register",
      "resetPassword",
      "tokens",
      "upload",
    ]);
  });
  it("匿名与登录状态均可打开画廊", async () => {
    const router = setupRouter();
    await router.push("/gallery");
    expect(router.currentRoute.value.path).toBe("/gallery");
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|token");
    useAuthStore().user = { id: 1, role: "user" } as never;
    await router.push("/upload");
    await router.push("/gallery");
    expect(router.currentRoute.value.path).toBe("/gallery");
  });
});

describe("经典布局标签页", () => {
  it("图片库路由按路径区分标签页，搜索条件变化不新开标签", () => {
    const router = setupRouter();
    for (const path of ["/images?q=a", "/albums/7?name=x&q=b"]) {
      const route = router.resolve(path);
      expect(route.meta.fullPathKey).toBe(false);
    }
  });
});
