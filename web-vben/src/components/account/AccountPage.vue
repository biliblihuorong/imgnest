<script setup lang="ts">
import { Page } from "@vben/common-ui";
import { useI18n } from "@vben/locales";

/** 账户页外壳：令牌与资料两个标签共用同一个标题、说明和标签栏。 */
const { t } = useI18n();

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
    </nav>
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
