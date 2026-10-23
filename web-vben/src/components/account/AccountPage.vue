<script setup lang="ts">
import { Page } from "@vben/common-ui";
import { useI18n } from "@vben/locales";
import { NAlert, NButton, NSpin } from "naive-ui";
import { useSiteStore } from "@/stores/site";
import { provideUserPlugins } from "./useUserPlugins";

/** 账户页外壳：通用标签与当前账户可用的个人设置入口。 */
const { t } = useI18n();
const site = useSiteStore();
const { plugins, loading, error, load } = provideUserPlugins();

const tabs = [
  { to: "/account/tokens", label: "account.tabs.tokens" },
  { to: "/account/settings", label: "account.tabs.profile" },
] as const;
</script>

<template>
  <Page
    class="account-page min-w-0"
    :description="t('account.description')"
    content-class="min-w-0"
  >
    <template #title>
      <h1 class="account-page__title">{{ t("account.title") }}</h1>
    </template>
    <template #extra><slot name="extra" /></template>
    <nav class="account-page__tabs" data-testid="account-tabs" :aria-label="t('account.title')">
      <RouterLink
        v-for="tab in tabs"
        :key="tab.to"
        :to="tab.to"
        class="account-page__tab"
        exact-active-class="is-active"
        >{{ t(tab.label) }}</RouterLink
      >
      <RouterLink
        v-if="site.billingEnabled"
        to="/account/billing"
        class="account-page__tab"
        exact-active-class="is-active"
        >{{ t("billing.title") }}</RouterLink
      >
      <RouterLink
        v-for="plugin in plugins"
        :key="plugin.name"
        :to="`/account/plugins/${encodeURIComponent(plugin.name)}`"
        class="account-page__tab"
        exact-active-class="is-active"
        >{{ plugin.title }}</RouterLink
      >
    </nav>
    <NSpin v-if="loading" size="small" />
    <NAlert v-if="error" type="error" class="mb-4">
      {{ error }} <NButton size="small" @click="load">{{ t("account.retry") }}</NButton>
    </NAlert>
    <slot />
  </Page>
</template>

<style scoped>
.account-page__title {
  margin: 0 0 8px;
  font-size: 20px;
  font-weight: 650;
}
.account-page__tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-bottom: 16px;
  border-bottom: 1px solid hsl(var(--border));
}
.account-page__tab {
  padding: 8px 12px;
  margin-bottom: -1px;
  font-size: 14px;
  color: hsl(var(--muted-foreground));
  border-bottom: 2px solid transparent;
}
.account-page__tab:hover {
  color: hsl(var(--foreground));
}
.account-page__tab.is-active {
  font-weight: 600;
  color: hsl(var(--primary));
  border-bottom-color: hsl(var(--primary));
}
</style>
