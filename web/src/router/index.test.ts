import { beforeEach, describe, expect, it, vi } from "vitest";
import { TOKEN_STORAGE_KEY } from "@/api/client";
import { ROUTER_NAMES, setupRouter } from "./index";

// 守卫测试只关心导航行为：用轻量 stub 替换真实视图模块，避免在测试阶段加载 naive-ui
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

describe("router 守卫", () => {
  beforeEach(() => {
    localStorage.clear();
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
    const router = setupRouter();
    await router.push("/login");
    await router.isReady();

    expect(router.currentRoute.value.path).toBe("/upload");
  });

  it("已登录访问受保护路由时放行", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|token");
    const router = setupRouter();
    await router.push("/upload");
    await router.isReady();

    expect(router.currentRoute.value.path).toBe("/upload");
  });

  it("未知路径重定向到 /upload 并受守卫约束", async () => {
    const router = setupRouter();
    await router.push("/no-such-page");
    await router.isReady();

    expect(router.currentRoute.value.path).toBe("/login");
    expect(router.currentRoute.value.query.redirect).toBe("/upload");
  });

  it("ROUTER_NAMES 暴露五个命名路由", () => {
    expect(Object.keys(ROUTER_NAMES).sort()).toEqual([
      "images",
      "login",
      "register",
      "tokens",
      "upload",
    ]);
  });
});
