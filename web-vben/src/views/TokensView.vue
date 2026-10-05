<script setup lang="ts">
import { Page } from "@vben/common-ui";
import { useI18n } from "@vben/locales";
import { NButton, NCard } from "naive-ui";
import { computed, onMounted, shallowRef } from "vue";
import { listTokens, type TokenView } from "@/api/tokens";
import ChangePasswordCard from "@/components/account/ChangePasswordCard.vue";
import CreateTokenCard from "@/components/account/CreateTokenCard.vue";
import TokenListCard from "@/components/account/TokenListCard.vue";
import TokenIntegrationHelp from "@/components/account/TokenIntegrationHelp.vue";
import { useAccountScope } from "@/components/account/useAccountScope";
import { formatApiError } from "@/locales/errors";

const { t, locale } = useI18n();
const tokens = shallowRef<TokenView[]>([]);
const loading = shallowRef(false);
const loadError = shallowRef<unknown>(null);
let reloadRequested = false;
const errorText = computed(() => {
  void locale.value;
  return loadError.value ? formatApiError(loadError.value, "account.loadError") : null;
});
const scope = useAccountScope(() => {
  tokens.value = [];
  loadError.value = null;
  reloadRequested = false;
});
async function loadTokens(): Promise<void> {
  if (!scope.available.value) return;
  if (loading.value) {
    // A mutation may finish during an older read. Coalesce, never lose, its refresh.
    reloadRequested = true;
    return;
  }
  loading.value = true;
  const version = scope.capture();
  try {
    do {
      reloadRequested = false;
      loadError.value = null;
      try {
        const result = await listTokens();
        if (scope.isCurrent(version) && !reloadRequested) tokens.value = result;
      } catch (error) {
        if (scope.isCurrent(version) && !reloadRequested) loadError.value = error;
      }
    } while (reloadRequested && scope.isCurrent(version));
  } finally {
    loading.value = false;
  }
}
onMounted(() => {
  void loadTokens();
});
</script>

<template>
  <Page class="tokens-view" :description="t('account.description')" header-class="flex-wrap gap-4">
    <template #title
      ><h1 class="tokens-view__title">{{ t("account.title") }}</h1></template
    >
    <template #extra
      ><NButton :loading="loading" :disabled="!scope.available.value" @click="loadTokens">{{
        t("account.refresh")
      }}</NButton></template
    >
    <div class="tokens-view__content">
      <CreateTokenCard @saved="loadTokens" />
      <TokenListCard :tokens="tokens" :loading="loading" :error="errorText" @refresh="loadTokens" />
      <TokenIntegrationHelp />
      <NCard :title="t('account.securityTitle')" :bordered="false">
        <p class="tokens-view__security-hint">{{ t("account.securityHint") }}</p>
        <ChangePasswordCard />
      </NCard>
    </div>
  </Page>
</template>
<style scoped>
.tokens-view__title {
  margin: 0 0 8px;
  font-size: 20px;
  font-weight: 650;
}
.tokens-view__content {
  display: grid;
  gap: 16px;
  min-width: 0;
}
.tokens-view__content > * {
  min-width: 0;
}
.tokens-view__security-hint {
  margin: 0 0 20px;
  color: hsl(var(--muted-foreground));
  font-size: 13px;
}
</style>
