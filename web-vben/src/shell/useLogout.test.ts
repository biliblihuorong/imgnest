import { flushPromises, mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { defineComponent, h } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";
import { beforeEach, expect, it, vi } from "vitest";
import { logout as logoutApi } from "@/api/auth";
import { useAuthStore } from "@/stores/auth";
import { useLogout } from "./useLogout";

vi.mock("@/api/auth", () => ({ logout: vi.fn(), login: vi.fn(), me: vi.fn() }));

beforeEach(() => {
  localStorage.clear();
  vi.resetAllMocks();
});

async function setup() {
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
  let api!: ReturnType<typeof useLogout>;
  const wrapper = mount(
    defineComponent({
      setup() {
        api = useLogout();
        return () => h("div");
      },
    }),
    { global: { plugins: [pinia, router] } },
  );
  return { api, router, complete, wrapper };
}

it("navigates to /login before the logout request settles", async () => {
  const { api, router, complete, wrapper } = await setup();
  const pending = api.logout();
  await flushPromises();
  expect(router.currentRoute.value.path).toBe("/login");
  expect(api.loggingOut.value).toBe(true);
  complete();
  await pending;
  expect(api.loggingOut.value).toBe(false);
  wrapper.unmount();
});

it("ignores a second call while logging out", async () => {
  const { api, complete, wrapper } = await setup();
  const first = api.logout();
  void api.logout();
  await flushPromises();
  expect(logoutApi).toHaveBeenCalledTimes(1);
  complete();
  await first;
  wrapper.unmount();
});
