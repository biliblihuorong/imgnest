import { mount, flushPromises, enableAutoUnmount } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { KeepAlive, h, shallowRef } from "vue";
import { NMessageProvider } from "naive-ui";
import { createRouter, createMemoryHistory } from "vue-router";
import { afterEach, expect, it, vi } from "vitest";
import * as api from "@/api/billing";
import type { UserView } from "@/api/types";
import { useAuthStore } from "@/stores/auth";
import AccountBilling from "./AccountBilling.vue";
vi.mock("@/api/billing", () => ({
  fetchBilling: vi.fn(),
  listBillingPlans: vi.fn(),
  listBillingLedger: vi.fn(),
  listBillingOrders: vi.fn(),
  purchasePackage: vi.fn(),
}));
enableAutoUnmount(afterEach);
it("aborts a cached page purchase and restores the original pending request on return", async () => {
  sessionStorage.clear();
  const pinia = createPinia();
  setActivePinia(pinia);
  const auth = useAuthStore();
  auth.token = "session";
  auth.user = { id: 7, role: "user", group_id: 1 } as UserView;
  vi.mocked(api.fetchBilling).mockResolvedValue({
    currency: "CNY",
    wallet: {
      user_id: 7,
      username: "Alice",
      balance_cents: 10000,
      version: 1,
      updated_at: "2026-10-23T00:00:00Z",
    },
    membership: null,
  });
  vi.mocked(api.listBillingPlans).mockResolvedValue([
    {
      id: 1,
      name: "Standard",
      description: "",
      price_cents: 3000,
      duration_days: 30,
      group_id: 2,
      published: true,
      archived: false,
      version: 1,
      created_at: "2026-10-23T00:00:00Z",
      updated_at: "2026-10-23T00:00:00Z",
    },
  ]);
  vi.mocked(api.listBillingLedger).mockResolvedValue({ items: [], total: 0, page: 1, size: 20 });
  vi.mocked(api.listBillingOrders).mockResolvedValue({ items: [], total: 0, page: 1, size: 20 });
  vi.mocked(api.purchasePackage).mockImplementation(() => new Promise(() => {}));
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/", component: { render: () => null } }],
  });
  await router.push("/");
  const active = shallowRef(true);
  const empty = { render: () => null };
  const wrapper = mount(
    () =>
      h(NMessageProvider, () =>
        h(KeepAlive, null, { default: () => (active.value ? h(AccountBilling) : h(empty)) }),
      ),
    { global: { plugins: [pinia, router] } },
  );
  await flushPromises();
  await wrapper.get('[data-buy-plan="1"]').trigger("click");
  await flushPromises();
  await wrapper.get("[data-confirm-purchase]").trigger("click");
  await flushPromises();
  const request = vi.mocked(api.purchasePackage).mock.calls[0]!;
  active.value = false;
  await flushPromises();
  expect(request[1]?.aborted).toBe(true);
  active.value = true;
  await flushPromises();
  await wrapper.get("[data-confirm-purchase]").trigger("click");
  await flushPromises();
  expect(vi.mocked(api.purchasePackage).mock.calls[1]?.[0].request_id).toBe(request[0].request_id);
});
