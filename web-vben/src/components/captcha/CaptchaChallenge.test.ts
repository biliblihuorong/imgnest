import { i18n } from "@vben/locales";
import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TurnstileOptions } from "./turnstile";
import CaptchaChallenge from "./CaptchaChallenge.vue";

let options: TurnstileOptions[];
const render = vi.fn((_element: HTMLElement, value: TurnstileOptions) => {
  options.push(value);
  return `widget-${options.length}`;
});
const reset = vi.fn();
const remove = vi.fn();
const config = {
  enabled: true,
  provider: "turnstile" as const,
  site_key: "public-key",
  version: 1,
};
beforeEach(() => {
  options = [];
  vi.clearAllMocks();
  window.turnstile = { render, reset, remove };
});
afterEach(() => {
  delete window.turnstile;
});

describe("CAPTCHA widget lifecycle", () => {
  it("renders the fixed action with no business fields and clears expired tokens", async () => {
    const wrapper = mount(CaptchaChallenge, { props: { config, action: "login" } });
    await flushPromises();
    expect(options[0]).toMatchObject({
      sitekey: "public-key",
      action: "login",
      language: "zh-CN",
      "response-field": false,
    });
    expect(options[0]).not.toHaveProperty("cData");
    options[0]!.callback("one-use");
    await flushPromises();
    expect(wrapper.emitted("token")?.at(-1)).toEqual(["one-use"]);
    options[0]!["expired-callback"]();
    await flushPromises();
    expect(wrapper.emitted("token")?.at(-1)).toEqual([null]);
    expect(wrapper.text()).toContain("验证码已失效");
    wrapper.unmount();
    expect(remove).toHaveBeenCalledWith("widget-1");
  });
  it("resets after consumption, rejects replay callbacks, and accepts a fresh token", async () => {
    const wrapper = mount(CaptchaChallenge, { props: { config, action: "login", resetKey: 0 } });
    await flushPromises();
    options[0]!.callback("used-token");
    await wrapper.setProps({ resetKey: 1 });
    expect(reset).toHaveBeenCalledWith("widget-1");
    options[0]!.callback("used-token");
    expect(wrapper.emitted("token")?.at(-1)).toEqual([null]);
    await flushPromises();
    await wrapper.find("button").trigger("click");
    options[0]!.callback("fresh-token");
    expect(wrapper.emitted("token")?.at(-1)).toEqual(["fresh-token"]);
    wrapper.unmount();
  });
  it("removes stale widgets on language/config/action changes and ignores old callbacks", async () => {
    const wrapper = mount(CaptchaChallenge, { props: { config, action: "login" } });
    await flushPromises();
    options[0]!.callback("old");
    i18n.global.locale.value = "en-US";
    await flushPromises();
    expect(remove).toHaveBeenCalledWith("widget-1");
    expect(options[1]?.language).toBe("en");
    options[0]!.callback("stale");
    expect(wrapper.emitted("token")?.at(-1)).toEqual([null]);
    await wrapper.setProps({
      config: { ...config, version: 2, site_key: "new-key" },
      action: "register",
    });
    await flushPromises();
    expect(options[2]).toMatchObject({ sitekey: "new-key", action: "register" });
    wrapper.unmount();
    options[2]!.callback("after-unmount");
    expect(wrapper.emitted("token")?.at(-1)).toEqual([null]);
  });
  it("shows a retryable provider failure and rejects over-budget tokens", async () => {
    const wrapper = mount(CaptchaChallenge, { props: { config, action: "register" } });
    await flushPromises();
    options[0]!.callback("x".repeat(2049));
    expect(wrapper.emitted("token")?.at(-1)).toEqual([null]);
    options[0]!["error-callback"]();
    await flushPromises();
    expect(wrapper.find('[role="alert"]').exists()).toBe(true);
    await wrapper.find("button").trigger("click");
    expect(reset).toHaveBeenCalledWith("widget-1");
    wrapper.unmount();
  });
  it("never renders an enabled configuration without its supported provider/key", async () => {
    const wrapper = mount(CaptchaChallenge, {
      props: { config: { ...config, provider: "" }, action: "login" },
    });
    await flushPromises();
    expect(render).not.toHaveBeenCalled();
    expect(wrapper.find('[role="alert"]').exists()).toBe(true);
    wrapper.unmount();
  });
  it("ignores delayed success after provider failure until an explicit retry", async () => {
    const wrapper = mount(CaptchaChallenge, { props: { config, action: "login" } });
    await flushPromises();
    options[0]!["error-callback"]();
    options[0]!.callback("late-after-failure");
    expect(wrapper.emitted("token")?.at(-1)).toEqual([null]);
    await flushPromises();
    await wrapper.find("button").trigger("click");
    options[0]!.callback("fresh-after-retry");
    expect(wrapper.emitted("token")?.at(-1)).toEqual(["fresh-after-retry"]);
    wrapper.unmount();
  });
  it("keeps a failed reset blocked even if the provider later calls success", async () => {
    const wrapper = mount(CaptchaChallenge, { props: { config, action: "login", resetKey: 0 } });
    await flushPromises();
    options[0]!.callback("before-reset");
    reset.mockImplementationOnce(() => {
      throw new Error("provider reset failed");
    });
    await wrapper.setProps({ resetKey: 1 });
    options[0]!.callback("late-after-reset-failure");
    expect(wrapper.emitted("token")?.at(-1)).toEqual([null]);
    expect(wrapper.find('[role="alert"]').exists()).toBe(true);
    wrapper.unmount();
  });
});
