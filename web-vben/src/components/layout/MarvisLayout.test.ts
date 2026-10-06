import { flushPromises, mount } from "@vue/test-utils";
import { updatePreferences } from "@vben/preferences";
import { createPinia } from "pinia";
import { nextTick, ref } from "vue";
import { beforeEach, expect, it, vi } from "vitest";
import { logout as logoutApi } from "@/api/auth";
import { useAuthStore } from "@/stores/auth";
import MarvisLayout from "./MarvisLayout.vue";
import MarvisSidebar from "./marvis/MarvisSidebar.vue";
import { shellTestRouter, testUser } from "./marvis/testing";

const narrow = ref(false);
vi.mock("./marvis/useNarrowViewport", () => ({ useViewportBelow: () => narrow }));
vi.mock("@/api/auth", () => ({ logout: vi.fn(), login: vi.fn(), me: vi.fn() }));
vi.mock("@/api/site", () => ({ fetchSite: vi.fn().mockRejectedValue(new Error("offline")) }));
vi.mock("@/api/albums", () => ({
  listAlbums: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, size: 20 }),
}));

beforeEach(() => {
  localStorage.clear();
  narrow.value = false;
  updatePreferences({ sidebar: { collapsed: false } });
  vi.mocked(logoutApi).mockResolvedValue(null as never);
});

async function mountLayout() {
  const pinia = createPinia();
  const auth = useAuthStore(pinia);
  auth.token = "token";
  auth.user = testUser("user");
  const router = shellTestRouter();
  await router.push("/upload");
  const wrapper = mount(MarvisLayout, { global: { plugins: [pinia, router] } });
  await flushPromises();
  return { wrapper, router };
}

it("renders the routed page next to the sidebar", async () => {
  const { wrapper } = await mountLayout();
  expect(wrapper.find('[data-testid="marvis-sidebar"]').exists()).toBe(true);
  expect(wrapper.find("main").exists()).toBe(true);
  expect(wrapper.find('[data-testid="marvis-menu-button"]').exists()).toBe(false);
});

it("closes the mobile drawer after navigation", async () => {
  narrow.value = true;
  const { wrapper, router } = await mountLayout();
  await wrapper.find('[data-testid="marvis-menu-button"]').trigger("click");
  expect(wrapper.find('[data-testid="marvis-sidebar"]').classes()).toContain("is-open");
  await router.push("/images");
  await flushPromises();
  expect(wrapper.find('[data-testid="marvis-sidebar"]').classes()).not.toContain("is-open");
});

it("follows sidebar.collapsed from Vben preferences on wide screens only", async () => {
  const { wrapper } = await mountLayout();
  updatePreferences({ sidebar: { collapsed: true } });
  await nextTick();
  expect(wrapper.findComponent(MarvisSidebar).props("collapsed")).toBe(true);
  narrow.value = true;
  await nextTick();
  expect(wrapper.findComponent(MarvisSidebar).props("collapsed")).toBe(false);
});

it("logs out through the shared logout flow", async () => {
  const { wrapper, router } = await mountLayout();
  await wrapper.find('[data-testid="sidebar-logout"]').trigger("click");
  await flushPromises();
  expect(logoutApi).toHaveBeenCalledTimes(1);
  expect(router.currentRoute.value.path).toBe("/login");
});
