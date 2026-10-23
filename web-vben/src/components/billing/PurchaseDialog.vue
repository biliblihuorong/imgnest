<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NAlert, NButton, NCard, NSpace } from "naive-ui";
import { onBeforeUnmount, shallowRef } from "vue";
import { ApiError } from "@/api/client";
import {
  purchasePackage,
  type BillingOrder,
  type BillingPlan,
  type BillingPurchase,
} from "@/api/billing";
import { formatApiError } from "@/locales/errors";
import { formatCents } from "./money";
import { clearPending, savePending, type PendingPurchase } from "./pendingPurchase";
const props = defineProps<{
  plan: BillingPlan;
  owner: number;
  balance: number;
  pending?: PendingPurchase | null;
}>();
const emit = defineEmits<{
  completed: [order: BillingOrder];
  close: [];
  refresh: [message: string];
}>();
const { t } = useI18n();
const busy = shallowRef(false),
  error = shallowRef(""),
  uncertain = shallowRef(Boolean(props.pending));
const controller = new AbortController();
const input: BillingPurchase = props.pending?.input ?? {
  plan_id: props.plan.id,
  plan_version: props.plan.version,
  request_id: crypto.randomUUID(),
};
onBeforeUnmount(() => controller.abort());
async function confirm() {
  if (busy.value || (props.balance < props.plan.price_cents && !uncertain.value)) return;
  busy.value = true;
  error.value = "";
  savePending(props.owner, { input, plan: props.plan });
  try {
    const order = await purchasePackage(input, controller.signal);
    if (controller.signal.aborted) return;
    clearPending(props.owner);
    emit("completed", order);
  } catch (cause) {
    if (controller.signal.aborted) return;
    error.value = formatApiError(cause, "billing.purchaseError");
    if (
      cause instanceof ApiError &&
      cause.status >= 400 &&
      cause.status < 500 &&
      cause.status !== 401 &&
      cause.status !== 429
    ) {
      clearPending(props.owner);
      uncertain.value = false;
      emit("refresh", error.value);
    } else uncertain.value = true;
  } finally {
    if (!controller.signal.aborted) busy.value = false;
  }
}
</script>
<template>
  <NCard
    :title="t('billing.confirmPurchase')"
    role="dialog"
    :aria-label="t('billing.confirmPurchase')"
    class="purchase-confirm"
  >
    <p>
      {{ plan.name }} · ¥{{ formatCents(plan.price_cents) }} /
      {{ t("billing.days", { count: plan.duration_days }) }}
    </p>
    <p>{{ t("billing.balanceAfter") }}: ¥{{ formatCents(balance - plan.price_cents) }}</p>
    <NAlert type="info">{{ t("billing.purchaseHint") }}</NAlert>
    <NAlert v-if="uncertain" type="warning" class="notice">{{ t("billing.uncertain") }}</NAlert>
    <NAlert v-if="error" type="error" class="notice">{{ error }}</NAlert>
    <NSpace class="actions"
      ><NButton
        type="primary"
        data-confirm-purchase
        :loading="busy"
        :disabled="busy"
        @click="confirm"
        >{{ t(uncertain ? "billing.retryPurchase" : "billing.confirmPurchase") }}</NButton
      ><NButton v-if="!uncertain" :disabled="busy" @click="$emit('close')">{{
        t("billing.cancel")
      }}</NButton></NSpace
    >
  </NCard>
</template>
<style scoped>
.purchase-confirm {
  margin-top: 20px;
  border-color: hsl(var(--primary));
}
.notice,
.actions {
  margin-top: 16px;
}
</style>
