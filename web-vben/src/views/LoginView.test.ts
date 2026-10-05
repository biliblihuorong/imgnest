import { i18n } from "@vben/locales";
import { flushPromises, mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter } from "vue-router";
import { ApiError, TOKEN_STORAGE_KEY } from "@/api/client";
import * as authApi from "@/api/auth";
import { fetchCaptcha } from "@/api/captcha";
import type { TurnstileOptions } from "@/components/captcha/turnstile";
import * as siteApi from "@/api/site";
import type { LoginData, UserView } from "@/api/types";
import { useAuthStore } from "@/stores/auth";
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

vi.mock("@/api/captcha", () => ({ fetchCaptcha: vi.fn() }));
const captchaConfig = vi.mocked(fetchCaptcha);

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
  pinia?: ReturnType<typeof createPinia>;
}

async function mountLoginView(options: MountOptions = {}) {
  const { registerEnabled = true, withToken = false, initialRoute = "/login" } = options;
  if (withToken) {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|token");
  }
  siteApiMock.fetchSite.mockResolvedValue({
    site_name: "测试图床",
    register_enabled: registerEnabled,
    gallery_enabled: true,
  });

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/login", component: LoginView },
      { path: "/register", component: { template: "<div />" } },
      { path: "/upload", component: { template: "<div />" } },
      { path: "/images", component: { template: "<div />" } },
      { path: "/dashboard", component: { template: "<div />" } },
    ],
  });
  await router.push(initialRoute);
  await router.isReady();

  const pinia = options.pinia ?? createPinia();
  const wrapper = mount(LoginView, {
    global: { plugins: [pinia, router] },
  });
  await flushPromises();
  return { wrapper, router, pinia };
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
    captchaConfig.mockResolvedValue({ enabled: false, provider: "", site_key: "", version: 0 });
    delete window.turnstile;
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

    expect(authApiMock.login).toHaveBeenCalledWith(
      "alice@example.com",
      "password-123",
      undefined,
      expect.any(AbortSignal),
    );
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

  it("即使开放注册，登录页也不展示创建账号入口", async () => {
    const { wrapper } = await mountLoginView({ registerEnabled: true });

    expect(wrapper.find("a[href='/register']").exists()).toBe(false);
  });

  it("管理员无 deep link 时进入概览", async () => {
    authApiMock.login.mockResolvedValue({
      token: "7|token",
      user: { ...userFixture, role: "admin" },
      expires_at: "2026-10-05T00:00:00Z",
    });
    const { wrapper, router } = await mountLoginView();
    await fillAndSubmit(wrapper);
    expect(router.currentRoute.value.path).toBe("/dashboard");
  });
  it("语言切换保留表单内容并翻译登录文案", async () => {
    const { wrapper } = await mountLoginView();
    await wrapper.find("input[placeholder='邮箱']").setValue("alice@example.com");
    i18n.global.locale.value = "en-US";
    await flushPromises();
    expect(wrapper.text()).toContain("Sign in");
    expect((wrapper.find("input[placeholder='Email']").element as HTMLInputElement).value).toBe(
      "alice@example.com",
    );
  });
  it("反斜杠和外部重定向不能离开应用", async () => {
    authApiMock.login.mockResolvedValue({
      token: "7|token",
      user: userFixture,
      expires_at: "2026-10-05T00:00:00Z",
    });
    const { wrapper, router } = await mountLoginView({
      initialRoute: "/login?redirect=" + encodeURIComponent("/\\evil.test"),
    });
    await fillAndSubmit(wrapper);
    expect(router.currentRoute.value.path).toBe("/upload");
  });
  it("cancels an unmounted login and ignores its eventual success and redirect", async () => {
    let finish!: (value: LoginData) => void;
    authApiMock.login.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const { wrapper, router, pinia } = await mountLoginView();
    await fillAndSubmit(wrapper);
    wrapper.unmount();
    await router.push("/register");
    finish({ token: "7|late", user: userFixture, expires_at: "2026-10-05T00:00:00Z" });
    await flushPromises();
    expect(useAuthStore(pinia).token).toBeNull();
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBeNull();
    expect(router.currentRoute.value.path).toBe("/register");
    expect(authApiMock.login.mock.calls[0]?.[3]?.aborted).toBe(true);
  });
  it("keeps a reopened login's account and destination after the first view completes late", async () => {
    let finishFirst!: (value: LoginData) => void;
    authApiMock.login.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finishFirst = resolve;
        }),
    );
    const first = await mountLoginView({ initialRoute: "/login?redirect=/images" });
    await fillAndSubmit(first.wrapper);
    first.wrapper.unmount();
    await first.router.push("/register");
    const secondUser = { ...userFixture, id: 2, username: "bob", role: "admin" as const };
    authApiMock.login.mockResolvedValueOnce({
      token: "8|second",
      user: secondUser,
      expires_at: "2026-10-05T00:00:00Z",
    });
    const second = await mountLoginView({ pinia: first.pinia });
    await fillAndSubmit(second.wrapper);
    finishFirst({ token: "7|first", user: userFixture, expires_at: "2026-10-05T00:00:00Z" });
    await flushPromises();
    expect(useAuthStore(first.pinia).user).toEqual(secondUser);
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBe("8|second");
    expect(first.router.currentRoute.value.path).toBe("/register");
    expect(second.router.currentRoute.value.path).toBe("/dashboard");
    second.wrapper.unmount();
  });
  it("preserves an in-flight valid login and its inputs when language changes", async () => {
    let finish!: (value: LoginData) => void;
    authApiMock.login.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const { wrapper, router } = await mountLoginView();
    await fillAndSubmit(wrapper);
    i18n.global.locale.value = "en-US";
    await flushPromises();
    expect((wrapper.get('input[aria-label="Password"]').element as HTMLInputElement).value).toBe(
      "password-123",
    );
    expect(authApiMock.login.mock.calls[0]?.[3]?.aborted).toBe(false);
    finish({
      token: "7|valid",
      user: { ...userFixture, role: "admin" },
      expires_at: "2026-10-05T00:00:00Z",
    });
    await flushPromises();
    expect(router.currentRoute.value.path).toBe("/dashboard");
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBe("7|valid");
    wrapper.unmount();
  });
});

describe("LoginView CAPTCHA integration", () => {
  let options: TurnstileOptions;
  beforeEach(() => {
    vi.resetAllMocks();
    localStorage.clear();
    captchaConfig.mockResolvedValue({
      enabled: true,
      provider: "turnstile",
      site_key: "public",
      version: 1,
    });
    window.turnstile = {
      render: vi.fn((_el, value) => {
        options = value;
        return "login-widget";
      }),
      reset: vi.fn(),
      remove: vi.fn(),
    };
  });
  it("blocks filled login when configuration is unavailable", async () => {
    captchaConfig.mockRejectedValue(new Error("offline"));
    const { wrapper } = await mountLoginView();
    await fillAndSubmit(wrapper);
    expect(authApiMock.login).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("无法读取验证设置");
    wrapper.unmount();
  });
  it("requires a token and consumes it exactly once even on repeated submit", async () => {
    authApiMock.login.mockImplementation(() => new Promise(() => {}));
    const { wrapper } = await mountLoginView();
    await fillAndSubmit(wrapper);
    expect(authApiMock.login).not.toHaveBeenCalled();
    options.callback("one-use-token");
    await flushPromises();
    await wrapper.find("form").trigger("submit");
    await wrapper.find("form").trigger("submit");
    expect(authApiMock.login).toHaveBeenCalledTimes(1);
    expect(authApiMock.login).toHaveBeenCalledWith(
      "alice@example.com",
      "password-123",
      "one-use-token",
      expect.any(AbortSignal),
    );
    expect(localStorage.length).toBe(0);
    wrapper.unmount();
  });
  it("clears failed or expired tokens and preserves inputs across language changes", async () => {
    authApiMock.login.mockRejectedValue(new ApiError(30010, "untrusted detail", 422));
    const { wrapper } = await mountLoginView();
    await fillAndSubmit(wrapper);
    options.callback("token-a");
    await flushPromises();
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(authApiMock.login).toHaveBeenCalledTimes(1);
    await wrapper.find("form").trigger("submit");
    expect(authApiMock.login).toHaveBeenCalledTimes(1);
    options.callback("token-b");
    options["expired-callback"]();
    await flushPromises();
    await wrapper.find("form").trigger("submit");
    expect(authApiMock.login).toHaveBeenCalledTimes(1);
    i18n.global.locale.value = "en-US";
    await flushPromises();
    expect((wrapper.find('input[aria-label="Email"]').element as HTMLInputElement).value).toBe(
      "alice@example.com",
    );
    expect((wrapper.find('input[aria-label="Password"]').element as HTMLInputElement).value).toBe(
      "password-123",
    );
    expect(options.language).toBe("en");
    wrapper.unmount();
  });
});
