import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { i18n } from "@vben/locales";
import { createMemoryHistory, createRouter } from "vue-router";
import { fetchDashboardMetric } from "@/api/dashboard";
import { useAuthStore } from "@/stores/auth";
import DashboardView from "./DashboardView.vue";
vi.mock("@/api/dashboard", () => ({ fetchDashboardMetric: vi.fn() }));
const fetchMetric = vi.mocked(fetchDashboardMetric);
enableAutoUnmount(afterEach);
const user = {
  id: 7,
  group_id: 1,
  username: "Reader",
  email: "reader@example.test",
  role: "user" as const,
  status: "enabled" as const,
  used_bytes: 99,
  created_at: "2026-01-01T00:00:00Z",
  display_name: "",
  avatar_provider: "weavatar" as const,
  avatar_url: null,
  avatar_config_version: 0,
};
async function mountView(role: "user" | "admin" = "user") {
  const pinia = createPinia();
  setActivePinia(pinia);
  const auth = useAuthStore();
  auth.token = "fixture-token";
  auth.user = { ...user, role };
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/:pathMatch(.*)*", component: { render: () => null } }],
  });
  await router.push("/dashboard");
  await router.isReady();
  const wrapper = mount(DashboardView, { global: { plugins: [pinia, router] } });
  await flushPromises();
  return { wrapper, auth };
}
beforeEach(() => {
  vi.resetAllMocks();
  fetchMetric.mockImplementation(
    async (metric) =>
      ({ storage: 2048, images: 1357, trash: 0, albums: 12, globalImages: 3456, accounts: 8 })[
        metric
      ],
  );
});
describe("DashboardView", () => {
  it("shows personal native counts without admin requests or made-up quota", async () => {
    const { wrapper } = await mountView();
    expect(wrapper.get('[data-metric="images"]').text()).toContain("1,357");
    expect(wrapper.get('[data-metric="storage"]').text()).toContain("2 KB");
    expect(wrapper.get('[data-metric="trash"]').text()).toContain("0");
    expect(fetchMetric.mock.calls.map(([key]) => key)).toEqual([
      "storage",
      "images",
      "albums",
      "trash",
    ]);
    expect(wrapper.find('[data-metric="accounts"]').exists()).toBe(false);
    expect(wrapper.text()).not.toMatch(/%|无限|unlimited/i);
  });
  it("shows genuine sitewide counts only for an admin and translates live", async () => {
    const { wrapper } = await mountView("admin");
    i18n.global.locale.value = "en-US";
    await flushPromises();
    expect(wrapper.get("h1").text()).toBe("Dashboard");
    expect(wrapper.get('[data-metric="globalImages"]').text()).toContain("3,456");
    expect(wrapper.get('[data-metric="accounts"]').text()).toContain("Total accounts");
    expect(wrapper.text()).not.toMatch(/today|uploaders|frequency/i);
  });
  it("keeps successful cards when one fails and retries only the failed metric", async () => {
    fetchMetric.mockImplementation(async (metric) => {
      if (metric === "albums") throw new Error("private diagnostics");
      return 42;
    });
    const { wrapper } = await mountView();
    expect(wrapper.get('[data-metric="images"]').text()).toContain("42");
    expect(wrapper.get('[data-metric="albums"]').text()).not.toContain("0");
    expect(wrapper.text()).not.toContain("private diagnostics");
    fetchMetric.mockResolvedValue(18);
    await wrapper.get('[data-metric="albums"] button').trigger("click");
    await flushPromises();
    expect(wrapper.get('[data-metric="albums"]').text()).toContain("18");
    expect(fetchMetric).toHaveBeenCalledTimes(5);
  });
  it("does not substitute zero while loading and discards stale results after identity change", async () => {
    let resolve!: (value: number) => void;
    fetchMetric.mockReturnValue(
      new Promise<number>((done) => {
        resolve = done;
      }),
    );
    const { wrapper, auth } = await mountView("admin");
    expect(wrapper.get('[data-metric="images"]').find("[data-value]").exists()).toBe(false);
    auth.clear();
    await flushPromises();
    resolve(98765);
    await flushPromises();
    expect(wrapper.text()).not.toContain("98,765");
    expect(wrapper.find('[data-metric="accounts"]').exists()).toBe(false);
  });
});
