import { mount, flushPromises, enableAutoUnmount } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { defineComponent, h } from "vue";
import { NMessageProvider } from "naive-ui";
import { createRouter, createMemoryHistory } from "vue-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "@/api/billing";
import { ApiError } from "@/api/client";
import { useAuthStore } from "@/stores/auth";
import type { UserView } from "@/api/types";
import AccountBilling from "./AccountBilling.vue";

vi.mock("@/api/billing", () => ({
  fetchBilling: vi.fn(),
  listBillingPlans: vi.fn(),
  listBillingLedger: vi.fn(),
  listBillingOrders: vi.fn(),
  purchasePackage: vi.fn(),
}));
enableAutoUnmount(afterEach);
const plan = {
  id: 1,
  name: "Standard",
  description: "Website uploads",
  price_cents: 3000,
  duration_days: 30,
  group_id: 2,
  version: 1,
  published: true,
  archived: false,
  created_at: "2026-10-23T00:00:00Z",
  updated_at: "2026-10-23T00:00:00Z",
};
const overview = {
  currency: "CNY" as const,
  wallet: {
    user_id: 7,
    username: "Alice",
    balance_cents: 10000,
    version: 1,
    updated_at: "2026-10-23T00:00:00Z",
  },
  membership: null,
};
const empty = { items: [], total: 0, page: 1, size: 20 };
beforeEach(() => {
  vi.resetAllMocks();
  sessionStorage.clear();
  localStorage.clear();
  vi.mocked(api.fetchBilling).mockResolvedValue(overview);
  vi.mocked(api.listBillingPlans).mockResolvedValue([plan]);
  vi.mocked(api.listBillingLedger).mockResolvedValue(empty);
  vi.mocked(api.listBillingOrders).mockResolvedValue(empty);
});
async function render() {
  const pinia = createPinia();
  setActivePinia(pinia);
  const auth = useAuthStore();
  auth.token = "session";
  auth.user = { id: 7, group_id: 1, role: "user" } as UserView;
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/", component: { render: () => null } },
      { path: "/login", component: { render: () => null } },
    ],
  });
  await router.push("/");
  const wrapper = mount(
    defineComponent({ render: () => h(NMessageProvider, () => h(AccountBilling)) }),
    { global: { plugins: [pinia, router] } },
  );
  await flushPromises();
  return { wrapper, auth, router };
}
describe("balance-funded package purchase", () => {
  it("shows private balance and disables unaffordable packages", async () => {
    vi.mocked(api.fetchBilling).mockResolvedValue({
      ...overview,
      wallet: { ...overview.wallet, balance_cents: 100 },
    });
    const { wrapper } = await render();
    expect(wrapper.text()).toContain("1.00");
    expect(wrapper.get('[data-buy-plan="1"]').attributes("disabled")).toBeDefined();
    expect(api.purchasePackage).not.toHaveBeenCalled();
  });
  it("retains the same request id after an uncertain response", async () => {
    vi.mocked(api.purchasePackage).mockRejectedValue(new ApiError(-1, "network", 0));
    const { wrapper } = await render();
    await wrapper.get('[data-buy-plan="1"]').trigger("click");
    await flushPromises();
    await wrapper.get("[data-confirm-purchase]").trigger("click");
    await flushPromises();
    await wrapper.get("[data-confirm-purchase]").trigger("click");
    await flushPromises();
    const calls = vi.mocked(api.purchasePackage).mock.calls;
    expect(calls).toHaveLength(2);
    expect(calls[0]?.[0].request_id).toBe(calls[1]?.[0].request_id);
    expect(sessionStorage.getItem("imgnest:billing:pending:7")).toContain(
      calls[0]?.[0].request_id ?? "missing",
    );
  });
  it("offers re-login after a committed group change without retrying the purchase", async () => {
    vi.mocked(api.purchasePackage).mockResolvedValue({
      currency: "CNY",
      id: 12,
      user_id: 7,
      username: "Alice",
      plan_id: 1,
      plan_name: "Standard",
      plan_version: 1,
      price_cents: 3000,
      duration_days: 30,
      group_id: 2,
      previous_expires_at: null,
      expires_at: "2026-11-22T00:00:00Z",
      requires_login: true,
      request_id: "success",
      created_at: "2026-10-23T00:00:00Z",
    });
    const { wrapper, auth, router } = await render();
    await wrapper.get('[data-buy-plan="1"]').trigger("click");
    await flushPromises();
    await wrapper.get("[data-confirm-purchase]").trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("购买成功");
    await wrapper.get("[data-billing-login]").trigger("click");
    await flushPromises();
    expect(auth.token).toBeNull();
    expect(router.currentRoute.value.path).toBe("/login");
    expect(api.purchasePackage).toHaveBeenCalledTimes(1);
  });
  it("discards a late response belonging to the previous account", async () => {
    let resolve!: (value: typeof overview) => void;
    vi.mocked(api.fetchBilling).mockImplementationOnce(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
    const { wrapper, auth } = await render();
    auth.user = { id: 8, group_id: 1, role: "user" } as UserView;
    await flushPromises();
    resolve({ ...overview, wallet: { ...overview.wallet, balance_cents: 999999 } });
    await flushPromises();
    expect(wrapper.text()).not.toContain("9999.99");
    expect(vi.mocked(api.fetchBilling).mock.calls[0]?.[0]?.aborted).toBe(true);
  });
  it("shows unavailable state when the server disables commerce", async () => {
    vi.mocked(api.fetchBilling).mockRejectedValue(new ApiError(10001, "not found", 404));
    const { wrapper } = await render();
    expect(wrapper.text()).toContain("当前不可用");
    expect(wrapper.find("[data-buy-plan]").exists()).toBe(false);
  });
  it("keeps a changed-offer explanation visible and requires a new confirmation", async () => {
    vi.mocked(api.purchasePackage).mockRejectedValue(
      new ApiError(30025, "changed", 409, { reason: "billing_conflict" }),
    );
    const { wrapper } = await render();
    await wrapper.get('[data-buy-plan="1"]').trigger("click");
    await flushPromises();
    await wrapper.get("[data-confirm-purchase]").trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("账务状态已变化");
    expect(wrapper.find("[data-confirm-purchase]").exists()).toBe(false);
    expect(api.purchasePackage).toHaveBeenCalledTimes(1);
  });
});
