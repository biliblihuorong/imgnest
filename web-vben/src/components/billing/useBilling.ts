import { onActivated, onBeforeUnmount, onDeactivated, shallowRef, watch } from "vue";
import {
  fetchBilling,
  listBillingPlans,
  type BillingOverview,
  type BillingPlan,
} from "@/api/billing";
import { ApiError } from "@/api/client";
import { useAuthStore } from "@/stores/auth";
import { formatApiError } from "@/locales/errors";

/** Private page state is scoped to the current account and active view. */
export function useBilling() {
  const auth = useAuthStore();
  const overview = shallowRef<BillingOverview | null>(null);
  const plans = shallowRef<BillingPlan[]>([]);
  const loading = shallowRef(false),
    unavailable = shallowRef(false),
    error = shallowRef("");
  let active = true,
    request: AbortController | undefined;
  function clear() {
    request?.abort();
    overview.value = null;
    plans.value = [];
    error.value = "";
    unavailable.value = false;
    loading.value = false;
  }
  async function load() {
    clear();
    if (!active || !auth.token || !auth.user) return;
    const controller = new AbortController();
    request = controller;
    loading.value = true;
    try {
      const [account, catalog] = await Promise.all([
        fetchBilling(controller.signal),
        listBillingPlans(controller.signal),
      ]);
      if (controller.signal.aborted || request !== controller) return;
      overview.value = account;
      plans.value = catalog;
    } catch (cause) {
      if (controller.signal.aborted || request !== controller) return;
      if (cause instanceof ApiError && cause.status === 404) unavailable.value = true;
      else error.value = formatApiError(cause, "billing.loadError");
    } finally {
      if (request === controller && !controller.signal.aborted) loading.value = false;
    }
  }
  watch(
    () => [auth.token, auth.user?.id, auth.user?.group_id],
    () => void load(),
    { immediate: true, flush: "sync" },
  );
  onDeactivated(() => {
    active = false;
    clear();
  });
  onActivated(() => {
    if (!active) {
      active = true;
      void load();
    }
  });
  onBeforeUnmount(() => {
    active = false;
    clear();
  });
  return { overview, plans, loading, unavailable, error, load };
}
