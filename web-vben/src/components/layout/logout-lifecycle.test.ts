import { flushPromises, mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { createMemoryHistory, createRouter } from "vue-router";
import { beforeEach, expect, it, vi } from "vitest";
import { useAuthStore } from "@/stores/auth";
import { logout as logoutApi } from "@/api/auth";
import AppLayout from "./AppLayout.vue";
vi.mock("@vben/layouts", () => ({
  BasicLayout: {
    emits: ["logout"],
    template: "<button @click=\"$emit('logout')\">Logout</button>",
  },
  UserDropdown: { template: "<span />" },
}));
vi.mock("@/api/auth", () => ({ logout: vi.fn(), login: vi.fn(), me: vi.fn() }));
vi.mock("@/api/site", () => ({
  fetchSite: vi
    .fn()
    .mockResolvedValue({ site_name: "ImageNest", register_enabled: false, gallery_enabled: false }),
}));
beforeEach(() => {
  localStorage.clear();
  vi.resetAllMocks();
});
it("a late logout response cannot navigate away from a newer account workspace", async () => {
  let complete!: () => void;
  vi.mocked(logoutApi).mockReturnValue(
    new Promise((resolve) => {
      complete = () => resolve(null);
    }),
  );
  const pinia = createPinia();
  const auth = useAuthStore(pinia);
  auth.token = "account-A";
  const view = { template: "<div />" };
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/images", component: view },
      { path: "/login", component: view },
    ],
  });
  await router.push("/images");
  const wrapper = mount(AppLayout, { global: { plugins: [pinia, router] } });
  await wrapper.find("button").trigger("click");
  auth.sessionGeneration++;
  auth.token = "account-B";
  await router.push("/images");
  complete();
  await flushPromises();
  expect(router.currentRoute.value.path).toBe("/images");
  expect(auth.token).toBe("account-B");
  wrapper.unmount();
});
