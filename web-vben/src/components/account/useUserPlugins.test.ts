/* eslint-disable vue/one-component-per-file -- KeepAlive needs distinct cached, inactive and host fixture components. */
import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { NMessageProvider } from "naive-ui";
import { defineComponent, h, KeepAlive, shallowRef } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";
import { afterEach, expect, it, vi } from "vitest";
import { listUserPluginSettings, type UserPluginSettings as Card } from "@/api/plugins";
import type { UserView } from "@/api/types";
import { useAuthStore } from "@/stores/auth";
import AccountPage from "./AccountPage.vue";
import UserPluginSettings from "./UserPluginSettings.vue";

vi.mock("@/api/plugins", async (original) => ({
  ...(await original<typeof import("@/api/plugins")>()),
  listUserPluginSettings: vi.fn(),
}));
enableAutoUnmount(afterEach);
it("cached account pages abort hidden requests and reload once when reactivated", async () => {
  localStorage.clear();
  vi.clearAllMocks();
  const pinia = createPinia();
  setActivePinia(pinia);
  const auth = useAuthStore();
  auth.token = "session";
  auth.user = { id: 1, group_id: 1 } as UserView;
  let late!: (cards: Card[]) => void;
  const card: Card = {
    name: "watermark",
    title: "我的水印",
    fields: [{ key: "text", label: "Text", type: "text" }],
    values: { text: "Current" },
    read_only: false,
    has_asset: false,
  };
  vi.mocked(listUserPluginSettings)
    .mockImplementationOnce(
      () =>
        new Promise((done) => {
          late = done;
        }),
    )
    .mockResolvedValueOnce([card]);
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/:pathMatch(.*)*", component: { render: () => null } }],
  });
  await router.push("/account/plugins/watermark");
  await router.isReady();
  const visible = shallowRef(true);
  const Panel = defineComponent({
    render: () => h(AccountPage, () => h(UserPluginSettings, { name: "watermark" })),
  });
  const Away = defineComponent({ render: () => h("div", "Away") });
  const Harness = defineComponent({
    render: () =>
      h(NMessageProvider, () => h(KeepAlive, () => (visible.value ? h(Panel) : h(Away)))),
  });
  const wrapper = mount(Harness, { global: { plugins: [pinia, router] } });
  await flushPromises();
  expect(listUserPluginSettings).toHaveBeenCalledTimes(1);
  visible.value = false;
  await flushPromises();
  expect(vi.mocked(listUserPluginSettings).mock.calls[0]?.[0]?.aborted).toBe(true);
  late([{ ...card, values: { text: "Late hidden data" } }]);
  await flushPromises();
  visible.value = true;
  await flushPromises();
  expect(listUserPluginSettings).toHaveBeenCalledTimes(2);
  expect((wrapper.get('input[aria-label="Text"]').element as HTMLInputElement).value).toBe(
    "Current",
  );
});
