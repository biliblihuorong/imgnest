<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "@vben/locales";
import { NButton, NCard, NSkeleton } from "naive-ui";
import type { DashboardMetric } from "@/api/dashboard";
import { formatApiError } from "@/locales/errors";
import type { MetricState } from "./useDashboard";
const props = defineProps<{ metric: DashboardMetric; state: MetricState }>();
defineEmits<{ retry: [] }>();
const { t, locale } = useI18n();
const valueText = computed(() => {
  if (props.state.value === null) return "";
  if (props.metric !== "storage")
    return new Intl.NumberFormat(locale.value).format(props.state.value);
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  let amount = props.state.value;
  let index = 0;
  while (amount >= 1024 && index < units.length - 1) {
    amount /= 1024;
    index += 1;
  }
  return t("dashboard.bytes", {
    value: new Intl.NumberFormat(locale.value, { maximumFractionDigits: 1 }).format(amount),
    unit: units[index],
  });
});
</script>
<template>
  <NCard :bordered="false" class="metric-card" :data-metric="metric" :aria-busy="state.loading">
    <h3 class="metric-card__title">{{ t(`dashboard.metrics.${metric}.title`) }}</h3>
    <div class="metric-card__body" aria-live="polite">
      <NSkeleton
        v-if="state.loading"
        text
        :width="110"
        :height="36"
        :aria-label="t('dashboard.loading')"
      />
      <div v-else-if="state.failed" class="metric-card__error">
        <p>{{ formatApiError(state.error, "dashboard.loadError") }}</p>
        <NButton size="small" secondary @click="$emit('retry')">{{ t("dashboard.retry") }}</NButton>
      </div>
      <p v-else-if="state.value !== null" class="metric-card__value" data-value>{{ valueText }}</p>
      <p v-else class="metric-card__unknown">{{ t("dashboard.unavailable") }}</p>
    </div>
    <p class="metric-card__description">{{ t(`dashboard.metrics.${metric}.description`) }}</p>
  </NCard>
</template>
<style scoped>
.metric-card {
  min-width: 0;
  height: 100%;
}
.metric-card__title {
  margin: 0;
  font-size: 13px;
  font-weight: 500;
  color: hsl(var(--muted-foreground));
}
.metric-card__body {
  min-height: 68px;
  display: flex;
  align-items: center;
}
.metric-card__value {
  margin: 12px 0;
  font-size: 30px;
  font-weight: 650;
  line-height: 1.25;
  letter-spacing: -0.03em;
  font-variant-numeric: tabular-nums;
}
.metric-card__description,
.metric-card__unknown {
  margin: 0;
  color: hsl(var(--muted-foreground));
  font-size: 12px;
  line-height: 1.7;
}
.metric-card__error {
  padding: 10px 0;
  font-size: 12px;
  color: hsl(var(--destructive));
}
.metric-card__error p {
  margin: 0 0 8px;
}
</style>
