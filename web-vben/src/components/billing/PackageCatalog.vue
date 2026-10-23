<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NButton, NCard, NEmpty } from "naive-ui";
import type { BillingPlan } from "@/api/billing";
import { formatBytes } from "@/lib/format";
import { formatCents } from "./money";
defineProps<{ plans: BillingPlan[]; balance: number; disabled?: boolean }>();
defineEmits<{ buy: [plan: BillingPlan] }>();
const { t } = useI18n();
</script>
<template>
  <section class="catalog" :aria-label="t('billing.packages')">
    <NEmpty v-if="!plans.length" :description="t('billing.noPackages')" />
    <NCard v-for="plan in plans" :key="plan.id" :title="plan.name" :bordered="false">
      <p class="price">
        ¥{{ formatCents(plan.price_cents) }}
        <span>/ {{ t("billing.days", { count: plan.duration_days }) }}</span>
      </p>
      <p>{{ plan.description }}</p>
      <div v-if="plan.group" class="benefits">
        <p>{{ plan.group.name }}</p>
        <p>
          {{ t("billing.storage") }}:
          {{
            plan.group.capacity_bytes
              ? formatBytes(plan.group.capacity_bytes)
              : t("billing.unlimited")
          }}
        </p>
        <p>
          {{ t("billing.maxFile") }}:
          {{
            plan.group.max_file_bytes
              ? formatBytes(plan.group.max_file_bytes)
              : t("billing.siteDefault")
          }}
        </p>
        <p>
          {{ t("billing.uploadRate") }}: {{ plan.group.upload_per_min || t("billing.unlimited") }}
        </p>
      </div>
      <NButton
        type="primary"
        :data-buy-plan="plan.id"
        :disabled="disabled || balance < plan.price_cents"
        @click="$emit('buy', plan)"
        >{{ t(balance < plan.price_cents ? "billing.insufficient" : "billing.buy") }}</NButton
      >
    </NCard>
  </section>
</template>
<style scoped>
.catalog {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 240px), 1fr));
  gap: 16px;
}
.price {
  font-size: 26px;
  font-weight: 650;
}
.price span {
  font-size: 13px;
  font-weight: 400;
  color: hsl(var(--muted-foreground));
}
.benefits {
  margin-bottom: 20px;
  font-size: 13px;
}
.benefits p {
  margin: 6px 0;
}
</style>
