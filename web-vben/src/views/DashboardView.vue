<script setup lang="ts">
import { computed } from "vue";
import { Page } from "@vben/common-ui";
import { useI18n } from "@vben/locales";
import { NButton, NCard } from "naive-ui";
import { RouterLink } from "vue-router";
import DashboardMetricCard from "@/components/dashboard/DashboardMetricCard.vue";
import { useDashboard } from "@/components/dashboard/useDashboard";
const { t } = useI18n();
const { isAdmin, metrics, keys, loading, refresh, refreshMetric } = useDashboard();
const personalKeys = computed(() =>
  keys.value.filter((key) => key !== "globalImages" && key !== "accounts"),
);
const adminKeys = computed(() =>
  keys.value.filter((key) => key === "globalImages" || key === "accounts"),
);
const links = computed(() => [
  { to: "/upload", key: "upload" },
  { to: "/images", key: "images" },
  { to: "/albums", key: "albums" },
  { to: "/tokens", key: "tokens" },
]);
</script>
<template>
  <Page
    class="dashboard-view"
    :description="t('dashboard.description')"
    header-class="flex-wrap gap-4"
  >
    <template #title
      ><h1 class="dashboard-view__title">{{ t("dashboard.title") }}</h1></template
    >
    <template #extra
      ><NButton :loading="loading" @click="refresh">{{ t("dashboard.refresh") }}</NButton></template
    >
    <div class="dashboard-view__content">
      <section :aria-label="t('dashboard.personal')">
        <h2 class="dashboard-view__section">{{ t("dashboard.personal") }}</h2>
        <div class="dashboard-view__metrics">
          <DashboardMetricCard
            v-for="key in personalKeys"
            :key="key"
            :metric="key"
            :state="metrics[key]"
            @retry="refreshMetric(key)"
          />
        </div>
      </section>
      <section v-if="isAdmin" :aria-label="t('dashboard.sitewide')">
        <h2 class="dashboard-view__section">{{ t("dashboard.sitewide") }}</h2>
        <div class="dashboard-view__metrics dashboard-view__metrics--admin">
          <DashboardMetricCard
            v-for="key in adminKeys"
            :key="key"
            :metric="key"
            :state="metrics[key]"
            @retry="refreshMetric(key)"
          />
        </div>
      </section>
      <NCard :title="t('dashboard.quickLinks')" :bordered="false">
        <nav class="dashboard-view__links" :aria-label="t('dashboard.quickLinks')">
          <RouterLink
            v-for="link in links"
            :key="link.to"
            :to="link.to"
            class="dashboard-view__link"
          >
            <span>{{ t(`dashboard.links.${link.key}.title`) }}</span>
            <span class="dashboard-view__link-description">{{
              t(`dashboard.links.${link.key}.description`)
            }}</span>
          </RouterLink>
        </nav>
      </NCard>
    </div>
  </Page>
</template>
<style scoped>
.dashboard-view__title {
  margin: 0 0 8px;
  font-size: 20px;
  font-weight: 650;
}
.dashboard-view__content {
  display: grid;
  gap: 24px;
  min-width: 0;
}
.dashboard-view__section {
  margin: 0 0 12px;
  font-size: 14px;
  font-weight: 600;
}
.dashboard-view__metrics {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}
.dashboard-view__metrics--admin {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}
.dashboard-view__links {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}
.dashboard-view__link {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 16px;
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
  color: hsl(var(--foreground));
  text-decoration: none;
  transition: border-color 150ms;
}
.dashboard-view__link:hover,
.dashboard-view__link:focus-visible {
  border-color: hsl(var(--primary));
  color: hsl(var(--primary));
}
.dashboard-view__link-description {
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}
@media (max-width: 1100px) {
  .dashboard-view__metrics,
  .dashboard-view__links {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (max-width: 560px) {
  .dashboard-view__metrics,
  .dashboard-view__links {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
