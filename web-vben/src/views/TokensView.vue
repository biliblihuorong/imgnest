<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NButton } from "naive-ui";
import { computed, onMounted, shallowRef } from "vue";
import { listTokens, type TokenView } from "@/api/tokens";
import AccountPage from "@/components/account/AccountPage.vue";
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
  <AccountPage class="tokens-view">
    <template #extra
      ><NButton :loading="loading" :disabled="!scope.available.value" @click="loadTokens">{{
        t("account.refresh")
      }}</NButton></template
    >
    <div class="tokens-view__content">
      <CreateTokenCard @saved="loadTokens" />
      <TokenListCard :tokens="tokens" :loading="loading" :error="errorText" @refresh="loadTokens" />
      <TokenIntegrationHelp />
    </div>
  </AccountPage>
</template>
<style scoped>
.tokens-view__content {
  display: grid;
  gap: 16px;
  min-width: 0;
}
.tokens-view__content > * {
  min-width: 0;
}
</style>
