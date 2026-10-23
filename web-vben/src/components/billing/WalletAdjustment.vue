<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NAlert, NButton, NCard, NSelect } from "naive-ui";
import { computed, onBeforeUnmount, onDeactivated, shallowRef, watch } from "vue";
import { ApiError } from "@/api/client";
import {
  adjustBillingWallet,
  reverseBillingEntry,
  type BillingAdjustment,
  type BillingReversal,
  type BillingEntry,
} from "@/api/billing";
import { useAuthStore } from "@/stores/auth";
import { formatApiError } from "@/locales/errors";
import { formatCents, parseCents } from "./money";
const props = defineProps<{ userId?: number; reverse?: BillingEntry | null }>();
const emit = defineEmits<{ saved: [entry: BillingEntry] }>();
const { t } = useI18n();
const auth = useAuthStore();
type Pending =
  | { kind: "adjust"; input: BillingAdjustment; entry?: never }
  | { kind: "reverse"; input: BillingReversal; entry: BillingEntry };
const target = shallowRef(String(props.userId ?? "")),
  amount = shallowRef(""),
  reason = shallowRef(""),
  direction = shallowRef("credit"),
  review = shallowRef(false),
  busy = shallowRef(false),
  pending = shallowRef<Pending | null>(null),
  reversal = shallowRef<BillingEntry | null>(null),
  error = shallowRef(""),
  success = shallowRef("");
let controller = new AbortController();
const storageKey = () => `imgnest:billing:adjustment:${auth.user?.id ?? 0}`;
function persist(value: Pending | null) {
  try {
    if (value) sessionStorage.setItem(storageKey(), JSON.stringify(value));
    else sessionStorage.removeItem(storageKey());
  } catch {
    /* The mounted form keeps the request key. */
  }
}
function restore() {
  controller.abort();
  controller = new AbortController();
  pending.value = null;
  reversal.value = null;
  review.value = false;
  busy.value = false;
  amount.value = "";
  reason.value = "";
  error.value = "";
  success.value = "";
  target.value = String(props.userId ?? "");
  if (!auth.user || auth.user.role !== "admin") return;
  try {
    const raw = sessionStorage.getItem(storageKey());
    if (raw) {
      const value = JSON.parse(raw) as Pending;
      if (
        (value.kind === "adjust" || value.kind === "reverse") &&
        value.input &&
        typeof value.input.request_id === "string" &&
        typeof value.input.reason === "string"
      ) {
        pending.value = value;
        review.value = true;
        reason.value = value.input.reason;
        if (value.kind === "adjust") {
          target.value = String(value.input.user_id);
          amount.value = formatCents(Math.abs(value.input.amount_cents));
          direction.value = value.input.amount_cents < 0 ? "debit" : "credit";
        } else reversal.value = value.entry;
      }
    }
  } catch {
    /* Ignore malformed local drafts. */
  }
}
watch(() => [auth.token, auth.user?.id], restore, { immediate: true, flush: "sync" });
watch(
  () => props.userId,
  (value) => {
    if (!pending.value && !review.value) target.value = String(value ?? "");
  },
);
watch(
  () => props.reverse,
  (value) => {
    if (!pending.value) {
      reversal.value = value ?? null;
      review.value = false;
      reason.value = "";
      error.value = "";
    }
  },
  { immediate: true },
);
onBeforeUnmount(() => controller.abort());
onDeactivated(() => controller.abort());
const cents = computed(() => parseCents(amount.value));
const valid = computed(
  () =>
    reason.value.trim().length > 0 &&
    reason.value.length <= 500 &&
    (Boolean(reversal.value) ||
      (/^[1-9]\d*$/.test(target.value) &&
        Number.isSafeInteger(Number(target.value)) &&
        cents.value !== null &&
        cents.value > 0)),
);
const previewAmount = computed(() =>
  reversal.value
    ? -reversal.value.amount_cents
    : (direction.value === "debit" ? -1 : 1) * (cents.value ?? 0),
);
async function confirm() {
  if (busy.value || !auth.user) return;
  const current =
    pending.value ??
    (reversal.value
      ? {
          kind: "reverse" as const,
          entry: reversal.value,
          input: {
            entry_id: reversal.value.id,
            reason: reason.value,
            request_id: crypto.randomUUID(),
          },
        }
      : {
          kind: "adjust" as const,
          input: {
            user_id: Number(target.value),
            amount_cents: previewAmount.value,
            reason: reason.value,
            request_id: crypto.randomUUID(),
          },
        });
  if (!pending.value && !valid.value) return;
  pending.value = current;
  persist(current);
  busy.value = true;
  error.value = "";
  success.value = "";
  try {
    const entry =
      current.kind === "adjust"
        ? await adjustBillingWallet(current.input, controller.signal)
        : await reverseBillingEntry(current.input, controller.signal);
    if (controller.signal.aborted) return;
    persist(null);
    pending.value = null;
    review.value = false;
    reversal.value = null;
    amount.value = "";
    reason.value = "";
    success.value = t("billing.adjustSuccess", { id: entry.id });
    emit("saved", entry);
  } catch (cause) {
    if (controller.signal.aborted) return;
    error.value = formatApiError(cause, "billing.saveError");
    if (
      cause instanceof ApiError &&
      cause.status >= 400 &&
      cause.status < 500 &&
      cause.status !== 401 &&
      cause.status !== 429
    ) {
      persist(null);
      pending.value = null;
      review.value = false;
    }
  } finally {
    if (!controller.signal.aborted) busy.value = false;
  }
}
</script>
<template>
  <NCard :title="t(reversal ? 'billing.reversal' : 'billing.adjustWallet')" :bordered="false">
    <p class="hint">{{ t(reversal ? "billing.reverseHint" : "billing.adjustHint") }}</p>
    <NAlert v-if="success" type="success">{{ success }}</NAlert
    ><NAlert v-if="error" type="error">{{ error }}</NAlert>
    <NAlert v-if="pending" type="warning">{{ t("billing.pendingAdjustment") }}</NAlert>
    <form class="adjust-form" @submit.prevent="review = valid">
      <template v-if="!reversal"
        ><label
          >{{ t("billing.targetUser")
          }}<input
            v-model="target"
            data-adjust-user
            inputmode="numeric"
            :disabled="Boolean(pending) || busy || review" /></label
        ><NSelect
          v-model:value="direction"
          :options="[
            { label: t('billing.credit'), value: 'credit' },
            { label: t('billing.debit'), value: 'debit' },
          ]"
          :disabled="Boolean(pending) || busy || review" /><label
          >{{ t("billing.amount")
          }}<input
            v-model="amount"
            data-adjust-amount
            inputmode="decimal"
            :disabled="Boolean(pending) || busy || review" /></label
      ></template>
      <p v-else>#{{ reversal.id }} · {{ reversal.username }} · ¥{{ formatCents(previewAmount) }}</p>
      <label
        >{{ t("billing.reason")
        }}<textarea
          v-model="reason"
          data-adjust-reason
          maxlength="500"
          :disabled="Boolean(pending) || busy || review"
        />
      </label>
      <NButton
        v-if="!review"
        attr-type="button"
        data-review-adjustment
        :disabled="!valid || busy"
        @click="review = valid"
        >{{ t(reversal ? "billing.confirmReverse" : "billing.confirmAdjustment") }}</NButton
      >
      <div v-else class="review">
        <p>
          {{ t("billing.user") }} #{{ reversal?.user_id ?? target }} · ¥{{
            formatCents(previewAmount)
          }}
          · {{ reason }}
        </p>
        <NButton
          type="primary"
          data-confirm-adjustment
          :loading="busy"
          :disabled="busy"
          @click="confirm"
          >{{ t(reversal ? "billing.confirmReverse" : "billing.confirmAdjustment") }}</NButton
        ><NButton v-if="!pending" class="cancel" @click="review = false">{{
          t("billing.cancel")
        }}</NButton>
      </div>
    </form>
  </NCard>
</template>
<style scoped>
.hint {
  color: hsl(var(--muted-foreground));
  line-height: 1.6;
}
.adjust-form {
  display: grid;
  gap: 14px;
  max-width: 600px;
  margin-top: 16px;
}
.adjust-form label {
  display: grid;
  gap: 6px;
}
.adjust-form input,
.adjust-form textarea {
  padding: 9px 12px;
  background: hsl(var(--background));
  border: 1px solid hsl(var(--border));
  border-radius: var(--radius);
  color: inherit;
  min-width: 0;
}
.adjust-form textarea {
  min-height: 76px;
}
.review {
  padding: 12px;
  border: 1px solid hsl(var(--primary));
  border-radius: var(--radius);
}
.cancel {
  margin-left: 10px;
}
</style>
