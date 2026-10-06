import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { createPinia } from "pinia";
import { defineComponent, h, ref } from "vue";
import { afterEach, beforeEach, expect, it } from "vitest";
import { useAuthStore } from "@/stores/auth";
import MarvisSettingsModal from "./MarvisSettingsModal.vue";
import { shellTestRouter, testUser } from "./testing";

let wrapper: VueWrapper | undefined;
beforeEach(() => {
  localStorage.clear();
});
afterEach(() => {
  wrapper?.unmount();
  wrapper = undefined;
  document.body.innerHTML = "";
});

const stubs = {
  AppearancePane: { template: '<div data-testid="pane-appearance" />' },
  ProfileCard: { template: '<div data-testid="pane-profile" />' },
  ChangePasswordCard: { template: '<div data-testid="pane-password" />' },
};
const q = (id: string) => document.body.querySelector<HTMLElement>(`[data-testid="${id}"]`);

async function mountModal(options: { anonymous?: boolean } = {}) {
  const pinia = createPinia();
  const auth = useAuthStore(pinia);
  if (!options.anonymous) {
    auth.token = "token";
    auth.user = testUser("user");
  }
  const router = shellTestRouter();
  await router.push("/upload");
  const show = ref(true);
  const Host = defineComponent({
    setup: () => () =>
      h(MarvisSettingsModal, {
        show: show.value,
        "onUpdate:show": (value: boolean) => (show.value = value),
      }),
  });
  wrapper = mount(Host, { attachTo: document.body, global: { plugins: [pinia, router], stubs } });
  await flushPromises();
  return { router, show };
}

it("shows the appearance pane first and switches panes", async () => {
  await mountModal();
  expect(q("pane-appearance")).not.toBeNull();
  expect(q("settings-tab-appearance")!.getAttribute("aria-selected")).toBe("true");
  q("settings-tab-account")!.click();
  await flushPromises();
  expect(q("pane-appearance")).toBeNull();
  expect(q("pane-profile")).not.toBeNull();
  expect(q("pane-password")).not.toBeNull();
});

it("closes itself when following the tokens link", async () => {
  const { router, show } = await mountModal();
  q("settings-tab-tokens")!.click();
  await flushPromises();
  q("settings-tokens-link")!.click();
  await flushPromises();
  expect(router.currentRoute.value.path).toBe("/tokens");
  expect(show.value).toBe(false);
  expect(q("settings-tab-appearance")).toBeNull();
});

it("closes from the close button and reopens on the appearance pane", async () => {
  const { show } = await mountModal();
  q("settings-tab-account")!.click();
  await flushPromises();
  q("settings-close")!.click();
  await flushPromises();
  expect(show.value).toBe(false);
  show.value = true;
  await flushPromises();
  expect(q("pane-appearance")).not.toBeNull();
});

it("offers only appearance to anonymous visitors", async () => {
  await mountModal({ anonymous: true });
  expect(q("settings-tab-appearance")).not.toBeNull();
  expect(q("settings-tab-account")).toBeNull();
  expect(q("settings-tab-tokens")).toBeNull();
});
