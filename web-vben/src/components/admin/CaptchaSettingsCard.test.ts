import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/api/client";
import * as api from "@/api/captcha";
import type { CaptchaAdminView } from "@/api/captcha";
import type { TurnstileOptions } from "@/components/captcha/turnstile";
import CaptchaSettingsCard from "./CaptchaSettingsCard.vue";

vi.mock("@/api/captcha", () => ({
  getCaptchaSettings: vi.fn(),
  configureCaptcha: vi.fn(),
}));
const apiMock = vi.mocked(api);
const candidate = {
  provider: "turnstile" as const,
  site_key: "public-key",
  hostnames: ["img.example.test"],
  secret_configured: true,
  version: 3,
};
const off: CaptchaAdminView = {
  version: 3,
  enabled: false,
  active: null,
  draft: null,
  configuration_available: true,
};
const on: CaptchaAdminView = { ...off, enabled: true, active: candidate };
let options: TurnstileOptions[];
const mounted: VueWrapper[] = [];

async function render(view = off) {
  apiMock.getCaptchaSettings.mockResolvedValue(view);
  const wrapper = mount(CaptchaSettingsCard);
  mounted.push(wrapper);
  await flushPromises();
  return wrapper;
}
function save(wrapper: VueWrapper) {
  return wrapper.get('button[data-action="save-captcha"]');
}
function siteKey(wrapper: VueWrapper) {
  return wrapper.get('input[aria-label="Site Key"]');
}
async function enable(wrapper: VueWrapper) {
  await wrapper.get("[data-captcha-enable]").trigger("click");
  await flushPromises();
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
  it("starts off with the current hostname and loads no provider script", async () => {
    const wrapper = await render();
    expect(wrapper.get("[data-captcha-status]").text()).toContain("未启用");
    expect((siteKey(wrapper).element as HTMLInputElement).value).toBe("");
    expect(wrapper.find("[data-captcha-verify]").exists()).toBe(false);
    expect(window.turnstile!.render).not.toHaveBeenCalled();
    expect(save(wrapper).attributes("disabled")).toBeDefined();
  });

  it("enables in one step: key, secret and one completed challenge", async () => {
    const wrapper = await render();
    await enable(wrapper);
    await siteKey(wrapper).setValue("new-key");
    await wrapper.get('input[type="password"]').setValue("new-secret");
    await flushPromises();
    expect(wrapper.find("[data-captcha-verify]").exists()).toBe(true);
    expect(options.at(-1)?.sitekey).toBe("new-key");
    expect(options.at(-1)?.action).toBe("login");
    expect(save(wrapper).attributes("disabled")).toBeDefined();
    options.at(-1)!.callback("token-1");
    await flushPromises();
    expect(save(wrapper).attributes("disabled")).toBeUndefined();
    apiMock.configureCaptcha.mockResolvedValue({
      ...on,
      version: 4,
      active: { ...candidate, site_key: "new-key", version: 4 },
    });
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(apiMock.configureCaptcha).toHaveBeenCalledWith({
      expected_version: 3,
      enabled: true,
      site_key: "new-key",
      hostnames: [window.location.hostname],
      secret: "new-secret",
      captcha_token: "token-1",
    });
    expect(wrapper.get("[data-captcha-status]").text()).toContain("已启用");
    expect((wrapper.get('input[type="password"]').element as HTMLInputElement).value).toBe("");
    expect(wrapper.text()).not.toContain("new-secret");
    expect(localStorage.length).toBe(0);
  });

  it("reuses the stored secret and requires a fresh challenge after editing the key", async () => {
    const wrapper = await render(on);
    expect((siteKey(wrapper).element as HTMLInputElement).value).toBe("public-key");
    expect(wrapper.get('input[type="password"]').attributes("placeholder")).toBe(
      "已保存，留空不修改",
    );
    await flushPromises();
    options.at(-1)!.callback("token-1");
    await flushPromises();
    expect(save(wrapper).attributes("disabled")).toBeUndefined();
    await siteKey(wrapper).setValue("other-key");
    await flushPromises();
    expect(save(wrapper).attributes("disabled")).toBeDefined();
    options.at(-1)!.callback("token-2");
    await flushPromises();
    apiMock.configureCaptcha.mockResolvedValue({ ...on, version: 4 });
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    const payload = apiMock.configureCaptcha.mock.calls[0]?.[0];
    expect(payload).toMatchObject({ site_key: "other-key", captcha_token: "token-2" });
    expect(payload).not.toHaveProperty("secret");
  });

  it("turns protection off without asking for a challenge", async () => {
    const wrapper = await render(on);
    await enable(wrapper);
    expect(wrapper.find("[data-captcha-verify]").exists()).toBe(false);
    apiMock.configureCaptcha.mockResolvedValue({ ...on, version: 4, enabled: false });
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(apiMock.configureCaptcha.mock.calls[0]?.[0]).toEqual({
      expected_version: 3,
      enabled: false,
      site_key: "public-key",
      hostnames: ["img.example.test"],
    });
    expect(wrapper.get("[data-captcha-status]").text()).toContain("未启用");
  });

  it("keeps edits and asks for a reload after a version conflict", async () => {
    const wrapper = await render(on);
    options.at(-1)!.callback("token-1");
    await flushPromises();
    apiMock.configureCaptcha.mockRejectedValue(new ApiError(30011, "stale", 409));
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(wrapper.find('button[data-action="reload"]').exists()).toBe(true);
    expect(save(wrapper).attributes("disabled")).toBeDefined();
    await siteKey(wrapper).setValue("kept-key");
    apiMock.getCaptchaSettings.mockResolvedValue({ ...on, version: 5 });
    await wrapper.get('button[data-action="reload"]').trigger("click");
    await flushPromises();
    expect((siteKey(wrapper).element as HTMLInputElement).value).toBe("kept-key");
    expect(wrapper.find('button[data-action="reload"]').exists()).toBe(false);
  });

  it("blocks configuration without a master key", async () => {
    const wrapper = await render({ ...off, configuration_available: false });
    expect(wrapper.text()).toContain("security.master_key");
    expect(siteKey(wrapper).attributes("disabled")).toBeDefined();
    expect(save(wrapper).attributes("disabled")).toBeDefined();
  });
});
