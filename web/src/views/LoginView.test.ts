import { flushPromises, mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter } from "vue-router";
import { ApiError, TOKEN_STORAGE_KEY } from "@/api/client";
import * as authApi from "@/api/auth";
import * as siteApi from "@/api/site";
import type { UserView } from "@/api/types";
import LoginView from "./LoginView.vue";

vi.mock("@/api/auth", () => ({
  login: vi.fn(),
  register: vi.fn(),
  logout: vi.fn(),
  me: vi.fn(),
  changePassword: vi.fn(),
}));

vi.mock("@/api/site", () => ({
  fetchSite: vi.fn(),
}));

const authApiMock = vi.mocked(authApi);
const siteApiMock = vi.mocked(siteApi);

const userFixture: UserView = {
  id: 1,
  group_id: 1,
  username: "alice",
  email: "alice@example.com",
  role: "user",
  status: "enabled",
  used_bytes: 0,
  created_at: "2026-10-04T00:00:00Z",
};

interface MountOptions {
  registerEnabled?: boolean;
  withToken?: boolean;
  initialRoute?: string;
}

async function mountLoginView(options: MountOptions = {}) {
  const { registerEnabled = true, withToken = false, initialRoute = "/login" } = options;
  if (withToken) {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|token");
  }
  siteApiMock.fetchSite.mockResolvedValue({
    site_name: "测试图床",
    register_enabled: registerEnabled,
    gallery_enabled: false,
  });

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/login", component: LoginView },
      { path: "/register", component: { template: "<div />" } },
      { path: "/upload", component: { template: "<div />" } },
      { path: "/images", component: { template: "<div />" } },
    ],
  });
  await router.push(initialRoute);
  await router.isReady();

  const wrapper = mount(LoginView, {
    global: { plugins: [createPinia(), router] },
  });
  await flushPromises();
  return { wrapper, router };
}

async function fillAndSubmit(wrapper: Awaited<ReturnType<typeof mountLoginView>>["wrapper"]) {
  await wrapper.find("input[placeholder='邮箱']").setValue("alice@example.com");
  await wrapper.find("input[placeholder='密码']").setValue("password-123");
  await wrapper.find("form").trigger("submit");
  await flushPromises();
}

describe("LoginView", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.resetAllMocks();
  });

  it("渲染邮箱、密码输入与登录按钮", async () => {
    const { wrapper } = await mountLoginView();

    expect(wrapper.find("input[placeholder='邮箱']").exists()).toBe(true);
    expect(wrapper.find("input[placeholder='密码']").exists()).toBe(true);
    expect(wrapper.text()).toContain("登录");
  });

  it("提交成功后跳转 redirect 参数指向的页面", async () => {
    authApiMock.login.mockResolvedValue({
      token: "7|token",
      user: userFixture,
      expires_at: "2026-10-05T00:00:00Z",
    });
    const { wrapper, router } = await mountLoginView({ initialRoute: "/login?redirect=/images" });

    await fillAndSubmit(wrapper);

    expect(authApiMock.login).toHaveBeenCalledWith("alice@example.com", "password-123");
    expect(router.currentRoute.value.path).toBe("/images");
  });

  it("提交成功且无 redirect 参数时跳转 /upload", async () => {
    authApiMock.login.mockResolvedValue({
      token: "7|token",
      user: userFixture,
      expires_at: "2026-10-05T00:00:00Z",
    });
    const { wrapper, router } = await mountLoginView();

    await fillAndSubmit(wrapper);

    expect(router.currentRoute.value.path).toBe("/upload");
  });

  it("提交失败时展示错误信息且停留在登录页", async () => {
    authApiMock.login.mockRejectedValue(new ApiError(20002, "邮箱或密码错误", 401));
    const { wrapper, router } = await mountLoginView();

    await fillAndSubmit(wrapper);

    expect(wrapper.text()).toContain("邮箱或密码错误");
    expect(router.currentRoute.value.path).toBe("/login");
  });

  it("registerEnabled=false 时不渲染注册链接", async () => {
    const { wrapper } = await mountLoginView({ registerEnabled: false });

    expect(wrapper.find("a[href='/register']").exists()).toBe(false);
  });

  it("registerEnabled=true 时渲染注册链接", async () => {
    const { wrapper } = await mountLoginView({ registerEnabled: true });

    expect(wrapper.find("a[href='/register']").exists()).toBe(true);
  });
});
