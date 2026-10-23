import { request } from "./client";
import type { components, operations } from "./schema";

export type BillingPlan = components["schemas"]["BillingPlan"];
export type BillingPlanInput = components["schemas"]["BillingPlanInput"];
export type BillingOverview = components["schemas"]["BillingOverview"];
export type BillingEntry = components["schemas"]["BillingEntry"];
export type BillingOrder = components["schemas"]["BillingOrder"];
export type BillingMembership = components["schemas"]["BillingMembership"];
export type BillingWallet = components["schemas"]["BillingWallet"];
export type BillingPurchase = components["schemas"]["BillingPurchase"];
export type BillingAdjustment = components["schemas"]["BillingAdjustment"];
export type BillingReversal = components["schemas"]["BillingReversal"];
export type EntryPage = components["schemas"]["BillingEntryPage"];
export type OrderPage = components["schemas"]["BillingOrderPage"];
export type WalletPage = components["schemas"]["BillingWalletPage"];
export type MembershipPage = components["schemas"]["BillingMembershipPage"];
export type BillingQuery = NonNullable<
  operations["billingAdminOrderHistory"]["parameters"]["query"]
>;
const root = "/api/user/billing";
const admin = "/api/admin/billing";
function query(input: BillingQuery = {}): string {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(input))
    if (value !== undefined) params.set(key, String(value));
  return params.size ? `?${params}` : "";
}
function write<T>(path: string, body: unknown, method = "POST", signal?: AbortSignal): Promise<T> {
  return request(path, {
    method,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    signal,
  });
}
export const fetchBilling = (signal?: AbortSignal): Promise<BillingOverview> =>
  request(root, { signal });
export const listBillingPlans = (signal?: AbortSignal): Promise<BillingPlan[]> =>
  request(`${root}/plans`, { signal });
export const listAdminBillingPlans = (signal?: AbortSignal): Promise<BillingPlan[]> =>
  request(`${admin}/plans`, { signal });
export const listBillingLedger = (
  params: BillingQuery = {},
  signal?: AbortSignal,
): Promise<EntryPage> => request(`${root}/ledger${query(params)}`, { signal });
export const listBillingOrders = (
  params: BillingQuery = {},
  signal?: AbortSignal,
): Promise<OrderPage> => request(`${root}/orders${query(params)}`, { signal });
export const listAdminBillingLedger = (
  params: BillingQuery = {},
  signal?: AbortSignal,
): Promise<EntryPage> => request(`${admin}/ledger${query(params)}`, { signal });
export const listAdminBillingOrders = (
  params: BillingQuery = {},
  signal?: AbortSignal,
): Promise<OrderPage> => request(`${admin}/orders${query(params)}`, { signal });
export const listBillingWallets = (
  params: BillingQuery = {},
  signal?: AbortSignal,
): Promise<WalletPage> => request(`${admin}/wallets${query(params)}`, { signal });
export const listBillingMemberships = (
  params: BillingQuery = {},
  signal?: AbortSignal,
): Promise<MembershipPage> => request(`${admin}/memberships${query(params)}`, { signal });
export const purchasePackage = (
  input: BillingPurchase,
  signal?: AbortSignal,
): Promise<BillingOrder> => write(`${root}/purchases`, input, "POST", signal);
export const adjustBillingWallet = (
  input: BillingAdjustment,
  signal?: AbortSignal,
): Promise<BillingEntry> => write(`${admin}/adjustments`, input, "POST", signal);
export const reverseBillingEntry = (
  input: BillingReversal,
  signal?: AbortSignal,
): Promise<BillingEntry> => write(`${admin}/reversals`, input, "POST", signal);
export const saveBillingPlan = (
  input: BillingPlanInput,
  id?: number,
  signal?: AbortSignal,
): Promise<BillingPlan> =>
  write(id ? `${admin}/plans/${id}` : `${admin}/plans`, input, id ? "PATCH" : "POST", signal);
