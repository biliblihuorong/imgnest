import type { BillingPlan, BillingPurchase } from "@/api/billing";
export interface PendingPurchase {
  input: BillingPurchase;
  plan: BillingPlan;
}
const key = (owner: number) => `imgnest:billing:pending:${owner}`;
export function readPending(owner: number): PendingPurchase | null {
  try {
    const raw = sessionStorage.getItem(key(owner));
    if (!raw) return null;
    const p = JSON.parse(raw) as PendingPurchase;
    if (
      !p.input ||
      !p.plan ||
      !Number.isSafeInteger(p.input.plan_id) ||
      p.input.plan_id < 1 ||
      p.input.plan_id !== p.plan.id ||
      p.input.plan_version !== p.plan.version ||
      typeof p.input.request_id !== "string" ||
      !/^[\w-]{1,100}$/.test(p.input.request_id) ||
      !Number.isSafeInteger(p.plan.price_cents) ||
      p.plan.price_cents < 1 ||
      p.plan.price_cents > 1_000_000_000_000
    )
      return null;
    return p;
  } catch {
    return null;
  }
}
export function savePending(owner: number, pending: PendingPurchase) {
  try {
    sessionStorage.setItem(key(owner), JSON.stringify(pending));
  } catch {
    /* The mounted dialog still retains its idempotency key. */
  }
}
export function clearPending(owner: number) {
  try {
    sessionStorage.removeItem(key(owner));
  } catch {
    /* No browser persistence was available. */
  }
}
