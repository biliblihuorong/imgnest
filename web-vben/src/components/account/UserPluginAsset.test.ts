import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { NMessageProvider } from "naive-ui";
import { defineComponent, h } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "@/api/plugins";
import UserPluginAsset from "./UserPluginAsset.vue";

vi.mock("@/api/plugins", () => ({
  getUserPluginAsset: vi.fn(),
  putUserPluginAsset: vi.fn(),
  deleteUserPluginAsset: vi.fn(),
}));
enableAutoUnmount(afterEach);
const create = vi.fn(() => "blob:private-logo");
const revoke = vi.fn();
beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal("URL", { createObjectURL: create, revokeObjectURL: revoke });
});
afterEach(() => vi.unstubAllGlobals());
function render() {
  return mount(
    defineComponent({
      render: () =>
        h(NMessageProvider, () =>
          h(UserPluginAsset, { name: "example", label: "Logo", hasAsset: true, disabled: false }),
        ),
    }),
  );
}
describe("private asset preview", () => {
  it("keeps the previous preview when replacement fails, then revokes on unmount", async () => {
    vi.mocked(api.getUserPluginAsset).mockResolvedValue(new Blob(["logo"]));
    vi.mocked(api.putUserPluginAsset).mockRejectedValue(new Error("bad logo"));
    const wrapper = render();
    await flushPromises();
    expect(wrapper.get("img").attributes("src")).toBe("blob:private-logo");
    const input = wrapper.get('input[type="file"]');
    Object.defineProperty(input.element, "files", { value: [new File(["bad"], "bad.png")] });
    await input.trigger("change");
    await flushPromises();
    expect(wrapper.get("img").attributes("src")).toBe("blob:private-logo");
    expect(revoke).not.toHaveBeenCalled();
    wrapper.unmount();
    expect(revoke).toHaveBeenCalledWith("blob:private-logo");
  });
  it("does not create a Blob URL after the account view has unmounted", async () => {
    let done!: (blob: Blob) => void;
    vi.mocked(api.getUserPluginAsset).mockImplementation(
      () =>
        new Promise((resolve) => {
          done = resolve;
        }),
    );
    const wrapper = render();
    await flushPromises();
    wrapper.unmount();
    done(new Blob(["late"]));
    await flushPromises();
    expect(create).not.toHaveBeenCalled();
    expect(vi.mocked(api.getUserPluginAsset).mock.calls[0]?.[1]?.aborted).toBe(true);
  });
  it("refreshes its private preview after a successful upload and removal", async () => {
    vi.mocked(api.getUserPluginAsset).mockResolvedValue(new Blob(["logo"]));
    vi.mocked(api.putUserPluginAsset).mockResolvedValue({
      has_asset: true,
    } as api.UserPluginSettings);
    vi.mocked(api.deleteUserPluginAsset).mockResolvedValue({
      has_asset: false,
    } as api.UserPluginSettings);
    const wrapper = render();
    await flushPromises();
    const input = wrapper.get('input[type="file"]');
    Object.defineProperty(input.element, "files", { value: [new File(["new"], "new.png")] });
    await input.trigger("change");
    await flushPromises();
    expect(api.getUserPluginAsset).toHaveBeenCalledTimes(2);
    await wrapper.get("button").trigger("click");
    await flushPromises();
    expect(wrapper.find("img").exists()).toBe(false);
  });
});
