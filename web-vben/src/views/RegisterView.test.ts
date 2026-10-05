import { i18n } from "@vben/locales";
import { flushPromises, mount } from "@vue/test-utils";
import { NMessageProvider } from "naive-ui";
import { createPinia } from "pinia";
import { defineComponent, h } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { register } from "@/api/auth";
import { fetchCaptcha } from "@/api/captcha";
import { fetchSite } from "@/api/site";
import type { UserView } from "@/api/types";
import type { TurnstileOptions } from "@/components/captcha/turnstile";
import RegisterView from "./RegisterView.vue";
vi.mock("@/api/auth", () => ({ register: vi.fn() }));
vi.mock("@/api/site", () => ({ fetchSite: vi.fn() }));
vi.mock("@/api/captcha", () => ({ fetchCaptcha: vi.fn() }));
const registerApi = vi.mocked(register);
let options: TurnstileOptions;

async function renderView(enabled = true) {
  vi.mocked(fetchSite).mockResolvedValue({
    site_name: "ImageNest",
    register_enabled: enabled,
    gallery_enabled: true,
  });
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/register", component: RegisterView },
      { path: "/login", component: { template: "<div />" } },
      { path: "/gallery", component: { template: "<div />" } },
    ],
  });
  await router.push("/register");
  await router.isReady();
  const wrapper = mount(
    defineComponent({
      setup: () => () => h(NMessageProvider, null, { default: () => h(RegisterView) }),
    }),
    { global: { plugins: [createPinia(), router] } },
  );
  await flushPromises();
  return { wrapper, router };
}
async function fill(wrapper: Awaited<ReturnType<typeof renderView>>["wrapper"]) {
  await wrapper.find('input[aria-label="用户名"]').setValue("alice");
  await wrapper.find('input[aria-label="邮箱"]').setValue("alice@example.test");
  await wrapper.find('input[aria-label="密码"]').setValue("password-12345");
  await wrapper.find('input[aria-label="确认密码"]').setValue("password-12345");
}
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(fetchCaptcha).mockResolvedValue({
    enabled: false,
    provider: "",
    site_key: "",
    version: 0,
  });
  window.turnstile = {
    render: vi.fn((_el, value) => {
      options = value;
      return "register-widget";
    }),
    reset: vi.fn(),
    remove: vi.fn(),
  };
});
describe("direct registration and CAPTCHA", () => {
  it("retains the direct registration route and exact disabled request", async () => {
    registerApi.mockResolvedValue({
      id: 2,
      group_id: 1,
      username: "alice",
      email: "alice@example.test",
      role: "user",
      status: "enabled",
      used_bytes: 0,
      created_at: "2026-10-05T00:00:00Z",
    });
    const { wrapper, router } = await renderView();
    await fill(wrapper);
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(registerApi).toHaveBeenCalledWith(
      {
        username: "alice",
        email: "alice@example.test",
        password: "password-12345",
      },
      expect.any(AbortSignal),
    );
    expect(router.currentRoute.value.path).toBe("/login");
    wrapper.unmount();
  });
  it("does not show the form or request challenges when registration is closed", async () => {
    const { wrapper } = await renderView(false);
    expect(wrapper.find("form").exists()).toBe(false);
    expect(registerApi).not.toHaveBeenCalled();
    expect(fetchCaptcha).not.toHaveBeenCalled();
    wrapper.unmount();
  });
  it("blocks registration when verification configuration cannot be read", async () => {
    vi.mocked(fetchCaptcha).mockRejectedValue(new Error("offline"));
    const { wrapper } = await renderView();
    await fill(wrapper);
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(registerApi).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("无法读取验证设置");
    wrapper.unmount();
  });
  it("requires the register action, handles failure without replay and preserves translated inputs", async () => {
    vi.mocked(fetchCaptcha).mockResolvedValue({
      enabled: true,
      provider: "turnstile",
      site_key: "public",
      version: 1,
    });
    registerApi.mockRejectedValue(new Error("private provider detail"));
    const { wrapper } = await renderView();
    await fill(wrapper);
    await wrapper.find("form").trigger("submit");
    expect(registerApi).not.toHaveBeenCalled();
    expect(options.action).toBe("register");
    options.callback("register-one-use");
    await flushPromises();
    await wrapper.find("form").trigger("submit");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(registerApi).toHaveBeenCalledTimes(1);
    expect(registerApi).toHaveBeenCalledWith(
      {
        username: "alice",
        email: "alice@example.test",
        password: "password-12345",
        captcha_token: "register-one-use",
      },
      expect.any(AbortSignal),
    );
    await wrapper.find("form").trigger("submit");
    expect(registerApi).toHaveBeenCalledTimes(1);
    expect(wrapper.text()).not.toContain("private provider detail");
    i18n.global.locale.value = "en-US";
    await flushPromises();
    expect((wrapper.find('input[aria-label="Email"]').element as HTMLInputElement).value).toBe(
      "alice@example.test",
    );
    expect((wrapper.find('input[aria-label="Password"]').element as HTMLInputElement).value).toBe(
      "password-12345",
    );
    wrapper.unmount();
  });
  it("does not redirect after a registration finishes following unmount", async () => {
    let finish!: (value: UserView) => void;
    registerApi.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const { wrapper, router } = await renderView();
    await fill(wrapper);
    await wrapper.find("form").trigger("submit");
    wrapper.unmount();
    await router.push("/gallery");
    finish({
      id: 2,
      group_id: 1,
      username: "alice",
      email: "alice@example.test",
      role: "user",
      status: "enabled",
      used_bytes: 0,
      created_at: "2026-10-05T00:00:00Z",
    });
    await flushPromises();
    expect(router.currentRoute.value.path).toBe("/gallery");
    expect(registerApi.mock.calls[0]?.[1]?.aborted).toBe(true);
  });
});
