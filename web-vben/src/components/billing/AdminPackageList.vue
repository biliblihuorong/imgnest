<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NButton, NCard, NEmpty, NTag } from "naive-ui";
import type { BillingPlan } from "@/api/billing";
import { formatCents } from "./money";
defineProps<{ plans: BillingPlan[] }>();
defineEmits<{ edit: [plan: BillingPlan] }>();
const { t } = useI18n();
</script>
<template>
  <NEmpty v-if="!plans.length" :description="t('billing.noPackages')" />
  <div class="plans">
    <NCard v-for="plan in plans" :key="plan.id" :title="plan.name" :bordered="false"
      ><NTag
        >{{ t(plan.archived ? "billing.archived" : "billing.published") }}:
        {{ plan.published ? "✓" : "—" }}</NTag
      >
      <p>
        ¥{{ formatCents(plan.price_cents) }} /
        {{ t("billing.days", { count: plan.duration_days }) }}
      </p>
      <p>{{ plan.group?.name ?? `#${plan.group_id}` }} · {{ plan.description }}</p>
      <NButton @click="$emit('edit', plan)">{{ t("billing.editPlan") }}</NButton></NCard
    >
  </div>
</template>
<style scoped>
.plans {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 260px), 1fr));
  gap: 16px;
}
</style>
