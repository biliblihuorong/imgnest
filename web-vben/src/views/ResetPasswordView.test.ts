import { flushPromises, mount } from "@vue/test-utils";
import { NMessageProvider } from "naive-ui";
import { createPinia } from "pinia";
import { defineComponent, h } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { resetPassword, sendEmailCode } from "@/api/auth";
import { fetchCaptcha } from "@/api/captcha";
import { ApiError } from "@/api/client";
import { fetchSite } from "@/api/site";
import ResetPasswordView from "./ResetPasswordView.vue";

vi.mock("@/api/auth", () => ({ resetPassword: vi.fn(), sendEmailCode: vi.fn() }));
vi.mock("@/api/site", () => ({ fetchSite: vi.fn() }));
vi.mock("@/api/captcha", () => ({ fetchCaptcha: vi.fn() }));

async function renderView(passwordReset = true) {
  vi.mocked(fetchSite).mockResolvedValue({
    site_name: "ImgNest",
    register_enabled: true,
    gallery_enabled: false,
    email_verification: false,
    password_reset: passwordReset,
    guest_upload: false,
    login_providers: [],
  });
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/reset-password", component: ResetPasswordView },
      { path: "/login", component: { template: "<div />" } },
    ],
  });
  await router.push("/reset-password");
  await router.isReady();
  const wrapper = mount(
    defineComponent({
      setup: () => () => h(NMessageProvider, null, { default: () => h(ResetPasswordView) }),
    }),
    { global: { plugins: [createPinia(), router] } },
  );
  await flushPromises();
  return { wrapper, router };
}

beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(fetchCaptcha).mockResolvedValue({
    enabled: false,
    provider: "",
    site_key: "",
    version: 0,
  });
  vi.mocked(sendEmailCode).mockResolvedValue(null);
});

describe("password reset", () => {
  it("says the feature is unavailable when the site cannot send mail", async () => {
    const { wrapper } = await renderView(false);
    expect(wrapper.find("form").exists()).toBe(false);
    expect(wrapper.text()).toContain("返回登录");
  });

  it("sends a reset code, then sets the new password and returns to sign-in", async () => {
    vi.mocked(resetPassword).mockResolvedValue(null);
    const { wrapper, router } = await renderView();
    const submit = () => wrapper.get('button[data-action="reset-password"]');
    await wrapper.get('input[aria-label="邮箱"]').setValue("alice@example.test");
    await wrapper.get('button[data-action="send-code"]').trigger("click");
    await flushPromises();
    expect(sendEmailCode).toHaveBeenCalledWith("alice@example.test", "reset_password", undefined);
    await wrapper.get('input[aria-label="邮箱验证码"]').setValue("654321");
    await wrapper.get('input[aria-label="新密码"]').setValue("new-password-123");
    await wrapper.get('input[aria-label="确认密码"]').setValue("new-password-12");
    expect(submit().attributes("disabled")).toBeDefined();
    await wrapper.get('input[aria-label="确认密码"]').setValue("new-password-123");
    expect(submit().attributes("disabled")).toBeUndefined();
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(resetPassword).toHaveBeenCalledWith("alice@example.test", "654321", "new-password-123");
    expect(router.currentRoute.value.path).toBe("/login");
    wrapper.unmount();
  });

  it("checks the new password length in UTF-8 bytes like the server", async () => {
    vi.mocked(resetPassword).mockResolvedValue(null);
    const { wrapper } = await renderView();
    const submit = () => wrapper.get('button[data-action="reset-password"]');
    await wrapper.get('input[aria-label="邮箱"]').setValue("alice@example.test");
    await wrapper.get('input[aria-label="邮箱验证码"]').setValue("654321");
    for (const [value, enabled] of [
      ["你好世", false],
      ["你好世界", true],
      ["abc你好世", true],
      ["你".repeat(24) + "a", false],
    ] as const) {
      await wrapper.get('input[aria-label="新密码"]').setValue(value);
      await wrapper.get('input[aria-label="确认密码"]').setValue(value);
      expect(submit().attributes("disabled") === undefined, value).toBe(enabled);
    }
    await wrapper.get('input[aria-label="新密码"]').setValue("你好世界");
    await wrapper.get('input[aria-label="确认密码"]').setValue("你好世界");
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(resetPassword).toHaveBeenCalledWith("alice@example.test", "654321", "你好世界");
    wrapper.unmount();
  });

  it("shows a wrong code and stays on the page", async () => {
    vi.mocked(resetPassword).mockRejectedValue(new ApiError(30018, "invalid", 422));
    const { wrapper, router } = await renderView();
    await wrapper.get('input[aria-label="邮箱"]').setValue("alice@example.test");
    await wrapper.get('input[aria-label="邮箱验证码"]').setValue("000000");
    await wrapper.get('input[aria-label="新密码"]').setValue("new-password-123");
    await wrapper.get('input[aria-label="确认密码"]').setValue("new-password-123");
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(wrapper.get('[role="alert"]').text()).toContain("验证码不正确或已过期");
    expect(router.currentRoute.value.path).toBe("/reset-password");
    wrapper.unmount();
  });

  it("shows the cooldown error from the server", async () => {
    vi.mocked(sendEmailCode).mockRejectedValue(new ApiError(30019, "cooldown", 429));
    const { wrapper } = await renderView();
    await wrapper.get('input[aria-label="邮箱"]').setValue("alice@example.test");
    await wrapper.get('button[data-action="send-code"]').trigger("click");
    await flushPromises();
    expect(wrapper.find('[role="alert"]').exists()).toBe(true);
    expect(wrapper.get('button[data-action="send-code"]').attributes("disabled")).toBeUndefined();
    wrapper.unmount();
  });
});
