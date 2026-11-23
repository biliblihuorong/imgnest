import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { NMessageProvider } from "naive-ui";
import { defineComponent, h } from "vue";
import { createMemoryHistory, createRouter, RouterView } from "vue-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "@/api/plugins";
import type { UserView } from "@/api/types";
import { useAuthStore } from "@/stores/auth";
import AccountPluginSettingsView from "./AccountPluginSettingsView.vue";

vi.mock("@/api/plugins", async (original) => ({
  ...(await original<typeof api>()),
  listUserPluginSettings: vi.fn(),
  saveUserPluginSettings: vi.fn(),
}));
enableAutoUnmount(afterEach);
const watermark: api.UserPluginSettings = {
  name: "watermark",
  title: "我的水印",
  fields: [{ key: "text", label: "文字水印", type: "text" }],
  values: { text: "Alice" },
  read_only: false,
  has_asset: false,
};
const other: api.UserPluginSettings = {
  ...watermark,
  name: "other",
  title: "其他偏好",
  values: { text: "Other" },
};

beforeEach(() => {
  vi.resetAllMocks();
  localStorage.clear();
  vi.mocked(api.listUserPluginSettings).mockResolvedValue([watermark, other]);
});
async function render(path = "/account/plugins/watermark") {
  const pinia = createPinia();
  setActivePinia(pinia);
  const auth = useAuthStore();
  auth.token = "fixture-session";
  auth.user = { id: 7, group_id: 1 } as UserView;
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/account/plugins/:plugin", component: AccountPluginSettingsView },
      { path: "/account/settings", component: { render: () => null } },
      { path: "/account/tokens", component: { render: () => null } },
    ],
  });
  await router.push(path);
  await router.isReady();
  const wrapper = mount(
    defineComponent({ render: () => h(NMessageProvider, () => h(RouterView)) }),
    { global: { plugins: [pinia, router] } },
  );
  await flushPromises();
  return { wrapper, router, auth };
}

describe("account personal settings route", () => {
  it("direct navigation shares one request between tabs and the selected card", async () => {
    const { wrapper } = await render();
    expect(wrapper.find('[data-user-plugin="watermark"]').exists()).toBe(true);
    expect(wrapper.find('[data-user-plugin="other"]').exists()).toBe(false);
    expect(wrapper.get('a[href="/account/plugins/watermark"]').attributes("aria-current")).toBe(
      "page",
    );
    expect(wrapper.find('input[placeholder="当前密码"]').exists()).toBe(false);
    expect(api.listUserPluginSettings).toHaveBeenCalledTimes(1);
  });
  it("changing the route discards a pending save from the previous card", async () => {
    let done!: (value: api.UserPluginSettings) => void;
    vi.mocked(api.saveUserPluginSettings).mockImplementation(
      () =>
        new Promise((resolve) => {
          done = resolve;
        }),
    );
    const { wrapper, router } = await render();
    await wrapper.get('input[aria-label="文字水印"]').setValue("Unfinished");
    await wrapper.get('[data-action="save-user-plugin"]').trigger("click");
    await router.push("/account/plugins/other");
    await flushPromises();
    done({ ...watermark, values: { text: "Old response" } });
    await flushPromises();
    expect(wrapper.find('[data-user-plugin="watermark"]').exists()).toBe(false);
    expect((wrapper.get('input[aria-label="文字水印"]').element as HTMLInputElement).value).toBe(
      "Other",
    );
    expect(vi.mocked(api.saveUserPluginSettings).mock.calls[0]?.[2]?.aborted).toBe(true);
  });
  it("unavailable settings have no extra tab or editable form", async () => {
    vi.mocked(api.listUserPluginSettings).mockResolvedValue([]);
    const { wrapper } = await render();
    expect(wrapper.text()).toContain("此设置当前不可用");
    expect(wrapper.findAll("[data-testid='account-tabs'] a")).toHaveLength(2);
    expect(wrapper.find("[data-user-plugin]").exists()).toBe(false);
    expect(wrapper.find('[data-action="save-user-plugin"]').exists()).toBe(false);
  });
  it("group-controlled users keep a visible entry and cannot edit", async () => {
    vi.mocked(api.listUserPluginSettings).mockResolvedValue([
      { ...watermark, read_only: true, notice: "当前用户组由管理员统一控制" },
    ]);
    const { wrapper } = await render();
    expect(wrapper.get('a[href="/account/plugins/watermark"]').text()).toBe("我的水印");
    expect(wrapper.text()).toContain("当前用户组由管理员统一控制");
    expect(wrapper.get('input[aria-label="文字水印"]').attributes("disabled")).toBeDefined();
    expect(wrapper.find('[data-action="save-user-plugin"]').exists()).toBe(false);
  });
  it("failed loading can be retried without rendering stale forms", async () => {
    vi.mocked(api.listUserPluginSettings)
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce([watermark]);
    const { wrapper } = await render();
    expect(wrapper.text()).toContain("加载个性设置失败");
    expect(wrapper.find("[data-user-plugin]").exists()).toBe(false);
    const retry = wrapper.findAll("button").find((button) => button.text() === "重试");
    await retry?.trigger("click");
    await flushPromises();
    expect(wrapper.find('[data-user-plugin="watermark"]').exists()).toBe(true);
    expect(wrapper.text()).not.toContain("加载个性设置失败");
  });
  it("switching accounts drops a previous account's navigation and card", async () => {
    let old!: (value: api.UserPluginSettings[]) => void;
    vi.mocked(api.listUserPluginSettings)
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            old = resolve;
          }),
      )
      .mockResolvedValueOnce([]);
    const { wrapper, auth } = await render();
    auth.user = { id: 9, group_id: 2 } as UserView;
    await flushPromises();
    old([watermark]);
    await flushPromises();
    expect(wrapper.text()).not.toContain("Alice");
    expect(wrapper.find('a[href="/account/plugins/watermark"]').exists()).toBe(false);
    expect(wrapper.find("[data-user-plugin]").exists()).toBe(false);
  });
});
