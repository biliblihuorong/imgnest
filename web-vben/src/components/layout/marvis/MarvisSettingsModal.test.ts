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

it("only holds appearance; account and tokens live on the account page", async () => {
  await mountModal();
  expect(q("pane-appearance")).not.toBeNull();
  expect(document.body.querySelector('[role="tab"]')).toBeNull();
});

it("links signed-in users to the account page and closes itself", async () => {
  const { router, show } = await mountModal();
  q("settings-account-link")!.click();
  await flushPromises();
  expect(router.currentRoute.value.path).toBe("/account");
  expect(show.value).toBe(false);
});

it("offers no account link to anonymous visitors", async () => {
  await mountModal({ anonymous: true });
  expect(q("pane-appearance")).not.toBeNull();
  expect(q("settings-account-link")).toBeNull();
});

it("closes from the close button", async () => {
  const { show } = await mountModal();
  q("settings-close")!.click();
  await flushPromises();
  expect(show.value).toBe(false);
});
