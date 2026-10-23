<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NCard, NStatistic, NTag } from "naive-ui";
import type { BillingOverview } from "@/api/billing";
import { formatDateTime } from "@/components/admin/format";
import { formatCents } from "./money";
defineProps<{ account: BillingOverview }>();
const { t } = useI18n();
</script>
<template>
  <NCard :title="t('billing.balance')" :bordered="false">
    <NStatistic
      :label="t('billing.availableBalance')"
      :value="formatCents(account.wallet.balance_cents)"
      ><template #prefix>¥</template></NStatistic
    >
    <p class="hint">{{ t("billing.manualCreditHint") }}</p>
    <template v-if="account.membership"
      ><NTag>{{ t(`billing.states.${account.membership.status}`) }}</NTag>
      <p>
        {{ t("billing.membershipGroup", { id: account.membership.group_id }) }} ·
        {{ t("billing.expiresAt") }} {{ formatDateTime(account.membership.expires_at) }}
      </p></template
    >
    <p v-else class="hint">{{ t("billing.noMembership") }}</p>
  </NCard>
</template>
<style scoped>
.hint {
  color: hsl(var(--muted-foreground));
  line-height: 1.6;
}
</style>
