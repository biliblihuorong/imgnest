import { computed, onBeforeUnmount, reactive, watch } from "vue";
import { fetchDashboardMetric, type DashboardMetric } from "@/api/dashboard";
import { useAuthStore } from "@/stores/auth";

export interface MetricState {
  value: number | null;
  loading: boolean;
  error: unknown;
  failed: boolean;
}
const emptyState = (): MetricState => ({ value: null, loading: false, error: null, failed: false });
const personal: DashboardMetric[] = ["storage", "images", "albums", "trash"];

export function useDashboard() {
  const auth = useAuthStore();
  const metrics = reactive<Record<DashboardMetric, MetricState>>({
    storage: emptyState(),
    images: emptyState(),
    albums: emptyState(),
    trash: emptyState(),
    globalImages: emptyState(),
    accounts: emptyState(),
  });
  const isAdmin = computed(() => Boolean(auth.token) && auth.user?.role === "admin");
  const keys = computed<DashboardMetric[]>(() =>
    !auth.token || !auth.user
      ? []
      : isAdmin.value
        ? [...personal, "globalImages", "accounts"]
        : personal,
  );
  const loading = computed(() => keys.value.some((key) => metrics[key].loading));
  let generation = 0;

  async function refreshMetric(key: DashboardMetric): Promise<void> {
    if (!auth.user || !keys.value.includes(key) || metrics[key].loading) return;
    const ownGeneration = generation;
    const identity = { id: auth.user.id, role: auth.user.role };
    metrics[key] = { value: null, loading: true, error: null, failed: false };
    try {
      const value = await fetchDashboardMetric(key, identity);
      if (ownGeneration === generation) metrics[key].value = value;
    } catch (error) {
      if (ownGeneration === generation) {
        metrics[key].error = error;
        metrics[key].failed = true;
      }
    } finally {
      if (ownGeneration === generation) metrics[key].loading = false;
    }
  }
  function refresh(): void {
    for (const key of keys.value) void refreshMetric(key);
  }
  watch(
    () => [auth.token, auth.user?.id, auth.user?.role],
    () => {
      generation += 1;
      for (const key of Object.keys(metrics) as DashboardMetric[]) metrics[key] = emptyState();
      refresh();
    },
    { immediate: true, flush: "sync" },
  );
  onBeforeUnmount(() => {
    generation += 1;
  });
  return { isAdmin, metrics, keys, loading, refresh, refreshMetric };
}
