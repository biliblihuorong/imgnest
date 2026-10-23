import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { NMessageProvider } from "naive-ui";
import { defineComponent, h } from "vue";
import { afterEach, expect, it, vi } from "vitest";
import { listUserPluginSettings, type UserPluginSettings as Card } from "@/api/plugins";
import type { UserView } from "@/api/types";
import { useAuthStore } from "@/stores/auth";
import UserPluginSettings from "./UserPluginSettings.vue";

vi.mock("@/api/plugins", async (original) => ({
  ...(await original<typeof import("@/api/plugins")>()),
  listUserPluginSettings: vi.fn(),
}));
enableAutoUnmount(afterEach);
it("discards a previous account's late settings response", async () => {
  const pinia = createPinia();
  setActivePinia(pinia);
  const auth = useAuthStore();
  auth.token = "session";
  auth.user = { id: 1, group_id: 1 } as UserView;
  let old!: (cards: Card[]) => void;
  vi.mocked(listUserPluginSettings)
    .mockImplementationOnce(
      () =>
        new Promise((done) => {
          old = done;
        }),
    )
    .mockResolvedValueOnce([]);
  const wrapper = mount(
    defineComponent({ render: () => h(NMessageProvider, () => h(UserPluginSettings)) }),
    { global: { plugins: [pinia] } },
  );
  await flushPromises();
  auth.user = { id: 2, group_id: 1 } as UserView;
  await flushPromises();
  old([
    {
      name: "old",
      title: "Alice's private settings",
      fields: [],
      values: {},
      read_only: false,
      has_asset: false,
    },
  ]);
  await flushPromises();
  expect(wrapper.text()).not.toContain("Alice");
  expect(wrapper.find("[data-user-plugin]").exists()).toBe(false);
  expect(vi.mocked(listUserPluginSettings).mock.calls[0]?.[0]?.aborted).toBe(true);
});
