import { i18n } from "@vben/locales";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/api/client";
import * as api from "@/api/captcha";
import type { CaptchaAdminView } from "@/api/captcha";
import type { TurnstileOptions } from "@/components/captcha/turnstile";
import CaptchaSettingsCard from "./CaptchaSettingsCard.vue";

vi.mock("@/api/captcha", () => ({
  getCaptchaSettings: vi.fn(),
  saveCaptchaDraft: vi.fn(),
  testCaptchaDraft: vi.fn(),
  setCaptchaActivation: vi.fn(),
}));
const apiMock = vi.mocked(api);
const candidate = {
  provider: "turnstile" as const,
  site_key: "public-key",
  hostnames: ["img.example.test"],
  secret_configured: true,
  version: 3,
};
const initial: CaptchaAdminView = {
  version: 3,
  enabled: false,
  active: null,
  draft: { ...candidate, tested: false, tested_actions: [] },
  configuration_available: true,
};
let options: TurnstileOptions[];
const mounted: VueWrapper[] = [];
async function render(view = initial) {
  apiMock.getCaptchaSettings.mockResolvedValue(view);
  const wrapper = mount(CaptchaSettingsCard);
  mounted.push(wrapper);
  await flushPromises();
  return wrapper;
}
function button(wrapper: VueWrapper, action: string) {
  return wrapper.get(`button[data-action="${action}"]`);
}
async function acknowledge(wrapper: VueWrapper) {
  await wrapper.get('[data-ack="legacy"]').trigger("click");
  await wrapper.get('[data-ack="v1"]').trigger("click");
}
beforeEach(() => {
  vi.resetAllMocks();
  localStorage.clear();
  options = [];
  window.turnstile = {
    render: vi.fn((_el, value) => {
      options.push(value);
      return `widget-${options.length}`;
    }),
    reset: vi.fn(),
    remove: vi.fn(),
  };
});
afterEach(() => {
  mounted.splice(0).forEach((wrapper) => wrapper.unmount());
  delete window.turnstile;
});

describe("CAPTCHA administrator controls", () => {
  it("loads the draft without echoing secrets or loading provider scripts before a test is requested", async () => {
    const wrapper = await render();
    expect((wrapper.get('input[aria-label="Site key"]').element as HTMLInputElement).value).toBe(
      "public-key",
    );
    expect((wrapper.get('input[type="password"]').element as HTMLInputElement).value).toBe("");
    expect(wrapper.text()).toContain("密钥已配置");
    expect(window.turnstile!.render).not.toHaveBeenCalled();
    expect(button(wrapper, "activate").attributes("disabled")).toBeDefined();
    expect(apiMock.setCaptchaActivation).not.toHaveBeenCalled();
  });
  it("omits a blank secret to reuse the configured secret, uses the current version, and invalidates tested actions after saving", async () => {
    const wrapper = await render({
      ...initial,
      draft: { ...initial.draft!, tested: true, tested_actions: ["login", "register"] },
    });
    await wrapper.get('input[aria-label="Site key"]').setValue("updated-key");
    apiMock.saveCaptchaDraft.mockResolvedValue({
      ...initial,
      version: 4,
      draft: { ...initial.draft!, site_key: "updated-key", version: 4 },
    });
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(apiMock.saveCaptchaDraft).toHaveBeenCalledWith(
      {
        expected_version: 3,
        provider: "turnstile",
        site_key: "updated-key",
        hostnames: ["img.example.test"],
      },
      expect.any(AbortSignal),
    );
    expect(button(wrapper, "activate").attributes("disabled")).toBeDefined();
    expect(apiMock.setCaptchaActivation).not.toHaveBeenCalled();
  });
  it("writes a replacement secret only in the save request and clears it after successful save", async () => {
    const wrapper = await render();
    await wrapper.get('input[type="password"]').setValue("replacement-secret");
    apiMock.saveCaptchaDraft.mockResolvedValue({ ...initial, version: 4 });
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(apiMock.saveCaptchaDraft.mock.calls[0]?.[0]).toEqual({
      expected_version: 3,
      provider: "turnstile",
      site_key: "public-key",
      hostnames: ["img.example.test"],
      secret: "replacement-secret",
    });
    expect((wrapper.get('input[type="password"]').element as HTMLInputElement).value).toBe("");
    expect(localStorage.length).toBe(0);
    expect(wrapper.text()).not.toContain("replacement-secret");
  });
  it("requires both independently verified actions and both risk acknowledgements before activation", async () => {
    const wrapper = await render();
    await acknowledge(wrapper);
    expect(button(wrapper, "activate").attributes("disabled")).toBeDefined();
    await button(wrapper, "start-login").trigger("click");
    await flushPromises();
    expect(options.at(-1)?.action).toBe("login");
    expect(options.at(-1)?.size).toBe("compact");
    options.at(-1)!.callback("login-test-token");
    await flushPromises();
    apiMock.testCaptchaDraft.mockResolvedValueOnce({
      ...initial,
      version: 4,
      draft: { ...initial.draft!, tested_actions: ["login"] },
    });
    await button(wrapper, "verify-login").trigger("click");
    await flushPromises();
    expect(apiMock.testCaptchaDraft.mock.calls[0]?.[0]).toEqual({
      expected_version: 3,
      captcha_token: "login-test-token",
      action: "login",
    });
    expect(wrapper.get('[data-action-state="login"]').text()).toContain("已通过");
    expect(button(wrapper, "activate").attributes("disabled")).toBeDefined();
    await button(wrapper, "start-register").trigger("click");
    await flushPromises();
    expect(options.at(-1)?.action).toBe("register");
    options.at(-1)!.callback("register-test-token");
    await flushPromises();
    apiMock.testCaptchaDraft.mockResolvedValueOnce({
      ...initial,
      version: 5,
      draft: { ...initial.draft!, tested: true, tested_actions: ["login", "register"] },
    });
    await button(wrapper, "verify-register").trigger("click");
    await flushPromises();
    expect(apiMock.testCaptchaDraft.mock.calls[1]?.[0]).toEqual({
      expected_version: 4,
      captcha_token: "register-test-token",
      action: "register",
    });
    expect(apiMock.setCaptchaActivation).not.toHaveBeenCalled();
    await wrapper.get('[data-ack="legacy"]').trigger("click");
    expect(button(wrapper, "activate").attributes("disabled")).toBeDefined();
    await wrapper.get('[data-ack="legacy"]').trigger("click");
    apiMock.setCaptchaActivation.mockResolvedValue({
      ...initial,
      version: 6,
      enabled: true,
      active: candidate,
      draft: { ...initial.draft!, tested: true, tested_actions: ["login", "register"] },
    });
    await button(wrapper, "activate").trigger("click");
    await flushPromises();
    expect(apiMock.setCaptchaActivation.mock.calls[0]?.[0]).toEqual({
      expected_version: 5,
      enabled: true,
      acknowledge_legacy_incompatibility: true,
      acknowledge_v1_unprotected: true,
    });
    expect(wrapper.get("[data-captcha-status]").text()).toContain("已启用");
  });
  it("blocks activation and testing while unsaved edits exist", async () => {
    const wrapper = await render({
      ...initial,
      draft: { ...initial.draft!, tested: true, tested_actions: ["login", "register"] },
    });
    await acknowledge(wrapper);
    expect(button(wrapper, "activate").attributes("disabled")).toBeUndefined();
    await wrapper.get('input[aria-label="Site key"]').setValue("unsaved-key");
    expect(button(wrapper, "activate").attributes("disabled")).toBeDefined();
    expect(button(wrapper, "start-login").attributes("disabled")).toBeDefined();
    await button(wrapper, "activate").trigger("click");
    expect(apiMock.setCaptchaActivation).not.toHaveBeenCalled();
  });
  it("consumes test tokens once, clears failures, and never disables protection on verification errors", async () => {
    const wrapper = await render({ ...initial, enabled: true, active: candidate });
    await button(wrapper, "start-login").trigger("click");
    await flushPromises();
    options.at(-1)!.callback("one-use");
    await flushPromises();
    apiMock.testCaptchaDraft.mockRejectedValue(
      new ApiError(50004, "secret provider diagnostic", 503),
    );
    await button(wrapper, "verify-login").trigger("click");
    await button(wrapper, "verify-login").trigger("click");
    await flushPromises();
    expect(apiMock.testCaptchaDraft).toHaveBeenCalledTimes(1);
    expect(apiMock.setCaptchaActivation).not.toHaveBeenCalled();
    expect(wrapper.get("[data-captcha-status]").text()).toContain("已启用");
    expect(wrapper.text()).not.toContain("secret provider diagnostic");
    expect(button(wrapper, "verify-login").attributes("disabled")).toBeDefined();
  });
  it("blocks stale writes after a conflict and reloads metadata without losing edits or retrying the write", async () => {
    const wrapper = await render();
    await wrapper.get('input[aria-label="Site key"]').setValue("my-edits");
    apiMock.saveCaptchaDraft.mockRejectedValue(new ApiError(30011, "private conflict detail", 409));
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(button(wrapper, "save").attributes("disabled")).toBeDefined();
    apiMock.getCaptchaSettings.mockResolvedValue({ ...initial, version: 9 });
    await button(wrapper, "reload").trigger("click");
    await flushPromises();
    expect((wrapper.get('input[aria-label="Site key"]').element as HTMLInputElement).value).toBe(
      "my-edits",
    );
    expect(apiMock.saveCaptchaDraft).toHaveBeenCalledTimes(1);
    apiMock.saveCaptchaDraft.mockResolvedValue({
      ...initial,
      version: 10,
      draft: { ...initial.draft!, site_key: "my-edits", version: 10 },
    });
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(apiMock.saveCaptchaDraft.mock.calls[1]?.[0].expected_version).toBe(9);
  });
  it("allows explicit secret clearing only while disabled and requires an explicit disable confirmation", async () => {
    const wrapper = await render({ ...initial, enabled: true, active: candidate });
    expect(wrapper.get("[data-clear-secret]").attributes("aria-disabled")).toBe("true");
    await button(wrapper, "disable").trigger("click");
    expect(apiMock.setCaptchaActivation).not.toHaveBeenCalled();
    await button(wrapper, "cancel-disable").trigger("click");
    expect(apiMock.setCaptchaActivation).not.toHaveBeenCalled();
    await button(wrapper, "disable").trigger("click");
    apiMock.setCaptchaActivation.mockResolvedValue({ ...initial, version: 4, active: candidate });
    await button(wrapper, "confirm-disable").trigger("click");
    await flushPromises();
    expect(apiMock.setCaptchaActivation.mock.calls[0]?.[0]).toEqual({
      expected_version: 3,
      enabled: false,
      acknowledge_legacy_incompatibility: false,
      acknowledge_v1_unprotected: false,
    });
    await wrapper.get("[data-clear-secret]").trigger("click");
    apiMock.saveCaptchaDraft.mockResolvedValue({
      ...initial,
      version: 5,
      draft: { ...initial.draft!, secret_configured: false, version: 5 },
    });
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(apiMock.saveCaptchaDraft.mock.calls[0]?.[0]).toEqual({
      expected_version: 4,
      provider: "turnstile",
      site_key: "public-key",
      hostnames: ["img.example.test"],
      clear_secret: true,
    });
  });
  it("keeps form values during a live language switch and blocks writes without server encryption support", async () => {
    const wrapper = await render();
    await wrapper.get('input[type="password"]').setValue("unsaved-secret");
    i18n.global.locale.value = "en-US";
    await flushPromises();
    expect(wrapper.text()).toContain("Human verification");
    expect((wrapper.get('input[type="password"]').element as HTMLInputElement).value).toBe(
      "unsaved-secret",
    );
    const unavailable = await render({ ...initial, configuration_available: false });
    expect(button(unavailable, "save").attributes("disabled")).toBeDefined();
    expect(button(unavailable, "activate").attributes("disabled")).toBeDefined();
    expect(button(unavailable, "start-login").attributes("disabled")).toBeDefined();
  });
  it("shows a retryable read error and aborts reads on unmount", async () => {
    apiMock.getCaptchaSettings.mockRejectedValue(
      new ApiError(50004, "private read diagnostic", 503),
    );
    const wrapper = mount(CaptchaSettingsCard);
    mounted.push(wrapper);
    await flushPromises();
    expect(wrapper.find('[role="alert"]').exists()).toBe(true);
    expect(wrapper.text()).not.toContain("private read diagnostic");
    apiMock.getCaptchaSettings.mockImplementation(() => new Promise(() => {}));
    await button(wrapper, "reload").trigger("click");
    const signal = apiMock.getCaptchaSettings.mock.calls.at(-1)?.[0];
    wrapper.unmount();
    expect(signal?.aborted).toBe(true);
  });
});
