import { mount, flushPromises, enableAutoUnmount } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "@/api/billing";
import { ApiError } from "@/api/client";
import { useAuthStore } from "@/stores/auth";
import type { UserView } from "@/api/types";
import WalletAdjustment from "./WalletAdjustment.vue";
vi.mock("@/api/billing", () => ({ adjustBillingWallet: vi.fn(), reverseBillingEntry: vi.fn() }));
enableAutoUnmount(afterEach);
beforeEach(() => {
  vi.resetAllMocks();
  sessionStorage.clear();
  const pinia = createPinia();
  setActivePinia(pinia);
  const auth = useAuthStore();
  auth.token = "admin";
  auth.user = { id: 1, role: "admin" } as UserView;
});
describe("administrator wallet adjustment", () => {
  it("requires a positive exact amount and a reason before confirmation", async () => {
    const wrapper = mount(WalletAdjustment, { props: { userId: 7 } });
    await wrapper.get("[data-adjust-amount]").setValue("1.001");
    await wrapper.get("[data-adjust-reason]").setValue("deposit");
    expect(wrapper.get("[data-review-adjustment]").attributes("disabled")).toBeDefined();
    expect(api.adjustBillingWallet).not.toHaveBeenCalled();
    await wrapper.get("[data-adjust-amount]").setValue("10.01");
    await wrapper.get("[data-review-adjustment]").trigger("click");
    expect(api.adjustBillingWallet).not.toHaveBeenCalled();
    expect(wrapper.find("[data-confirm-adjustment]").exists()).toBe(true);
  });
  it("retries an uncertain entry with its original request and immutable fields", async () => {
    vi.mocked(api.adjustBillingWallet).mockRejectedValue(new ApiError(-1, "network", 0));
    const wrapper = mount(WalletAdjustment, { props: { userId: 7 } });
    await wrapper.get("[data-adjust-amount]").setValue("10.01");
    await wrapper.get("[data-adjust-reason]").setValue("manual credit");
    await wrapper.get("[data-review-adjustment]").trigger("click");
    await wrapper.get("[data-confirm-adjustment]").trigger("click");
    await flushPromises();
    await wrapper.get("[data-confirm-adjustment]").trigger("click");
    await flushPromises();
    const calls = vi.mocked(api.adjustBillingWallet).mock.calls;
    expect(calls).toHaveLength(2);
    expect(calls[0]?.[0]).toEqual(calls[1]?.[0]);
    expect(calls[0]?.[0].amount_cents).toBe(1001);
    expect(wrapper.get("[data-adjust-amount]").attributes("disabled")).toBeDefined();
  });
});
