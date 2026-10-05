import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fetchCaptcha } from "@/api/captcha";
import type { TurnstileOptions } from "./turnstile";
import AuthCaptcha from "./AuthCaptcha.vue";
vi.mock("@/api/captcha", () => ({ fetchCaptcha: vi.fn() }));
const fetchConfig = vi.mocked(fetchCaptcha);
let options: TurnstileOptions;
beforeEach(() => {
  vi.resetAllMocks();
  window.turnstile = {
    render: vi.fn((_element, value) => {
      options = value;
      return "id";
    }),
    reset: vi.fn(),
    remove: vi.fn(),
  };
});
afterEach(() => {
  delete window.turnstile;
});

describe("public CAPTCHA gate", () => {
  it("blocks on configuration failure until an explicit successful retry", async () => {
    fetchConfig
      .mockRejectedValueOnce(new Error("private error"))
      .mockResolvedValue({ enabled: false, provider: "", site_key: "", version: 0 });
    const wrapper = mount(AuthCaptcha, { props: { action: "login" } });
    await flushPromises();
    expect(wrapper.emitted("ready")?.at(-1)).toEqual([false]);
    expect(wrapper.text()).not.toContain("private error");
    expect(wrapper.find('[role="alert"]').exists()).toBe(true);
    await wrapper.find("button").trigger("click");
    await flushPromises();
    expect(wrapper.emitted("ready")?.at(-1)).toEqual([true]);
    expect(wrapper.vm.consume()).toBeUndefined();
    wrapper.unmount();
  });
  it("consumes an enabled token once and immediately blocks reuse", async () => {
    fetchConfig.mockResolvedValue({
      enabled: true,
      provider: "turnstile",
      site_key: "key",
      version: 1,
    });
    const wrapper = mount(AuthCaptcha, { props: { action: "register" } });
    await flushPromises();
    expect(wrapper.emitted("ready")?.at(-1)).toEqual([false]);
    options.callback("token");
    await flushPromises();
    expect(wrapper.emitted("ready")?.at(-1)).toEqual([true]);
    expect(wrapper.vm.consume()).toBe("token");
    expect(wrapper.emitted("ready")?.at(-1)).toEqual([false]);
    expect(() => wrapper.vm.consume()).toThrow();
    wrapper.unmount();
  });
  it("aborts a pending config read and ignores completion after unmount", async () => {
    let resolve!: (value: Awaited<ReturnType<typeof fetchCaptcha>>) => void;
    fetchConfig.mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
    const ready = vi.fn();
    const wrapper = mount(AuthCaptcha, { props: { action: "login", onReady: ready } });
    const signal = fetchConfig.mock.calls[0]?.[0];
    wrapper.unmount();
    expect(signal?.aborted).toBe(true);
    resolve({ enabled: false, provider: "", site_key: "", version: 0 });
    await flushPromises();
    expect(ready).not.toHaveBeenCalledWith(true);
  });
});
