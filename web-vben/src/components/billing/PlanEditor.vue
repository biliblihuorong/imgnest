<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NAlert, NButton, NCard, NInput, NInputNumber, NSelect, NSwitch } from "naive-ui";
import { computed, onBeforeUnmount, onDeactivated, reactive, shallowRef } from "vue";
import { saveBillingPlan, type BillingPlan } from "@/api/billing";
import type { GroupView } from "@/api/admin";
import { formatApiError } from "@/locales/errors";
import { parseCents, formatCents } from "./money";
const props = defineProps<{ plan?: BillingPlan | null; groups: GroupView[] }>();
const emit = defineEmits<{ saved: []; close: [] }>();
const { t } = useI18n();
const model = reactive({
  name: props.plan?.name ?? "",
  description: props.plan?.description ?? "",
  price: props.plan ? formatCents(props.plan.price_cents) : "",
  days: props.plan?.duration_days ?? 30,
  group: props.plan?.group_id ?? null,
  published: props.plan?.published ?? false,
  archived: props.plan?.archived ?? false,
});
const busy = shallowRef(false),
  error = shallowRef("");
const controller = new AbortController();
onBeforeUnmount(() => controller.abort());
onDeactivated(() => controller.abort());
const groups = computed(() =>
  props.groups
    .filter((group) => !group.is_guest)
    .map((group) => ({ label: group.name, value: group.id })),
);
const valid = computed(
  () =>
    model.name.trim().length > 0 &&
    model.name.length <= 100 &&
    model.description.length <= 2000 &&
    parseCents(model.price) !== null &&
    (parseCents(model.price) ?? 0) > 0 &&
    Number.isInteger(model.days) &&
    model.days >= 1 &&
    model.days <= 3650 &&
    model.group !== null,
);
async function save() {
  if (busy.value || !valid.value) return;
  busy.value = true;
  error.value = "";
  try {
    await saveBillingPlan(
      {
        version: props.plan?.version ?? 0,
        name: model.name,
        description: model.description,
        price_cents: parseCents(model.price)!,
        duration_days: model.days,
        group_id: model.group!,
        published: !model.archived && model.published,
        archived: model.archived,
      },
      props.plan?.id,
      controller.signal,
    );
    if (!controller.signal.aborted) emit("saved");
  } catch (cause) {
    if (!controller.signal.aborted) error.value = formatApiError(cause, "billing.saveError");
  } finally {
    if (!controller.signal.aborted) busy.value = false;
  }
}
</script>
<template>
  <NCard :title="t(plan ? 'billing.editPlan' : 'billing.newPlan')"
    ><NAlert type="info">{{ t("billing.soldTermsHint") }}</NAlert
    ><NAlert v-if="error" type="error">{{ error }}</NAlert>
    <form class="plan-form" @submit.prevent="save">
      <label
        >{{ t("billing.name")
        }}<NInput v-model:value="model.name" :maxlength="100" :disabled="busy" /></label
      ><label
        >{{ t("billing.description")
        }}<NInput
          v-model:value="model.description"
          type="textarea"
          :maxlength="2000"
          :disabled="busy" /></label
      ><label
        >{{ t("billing.price")
        }}<NInput v-model:value="model.price" inputmode="decimal" :disabled="busy" /></label
      ><label
        >{{ t("billing.duration")
        }}<NInputNumber
          v-model:value="model.days"
          :min="1"
          :max="3650"
          :precision="0"
          :disabled="busy" /></label
      ><label
        >{{ t("billing.group")
        }}<NSelect v-model:value="model.group" :options="groups" :disabled="busy" /></label
      ><label class="toggle"
        >{{ t("billing.published")
        }}<NSwitch v-model:value="model.published" :disabled="busy || model.archived" /></label
      ><label v-if="plan" class="toggle"
        >{{ t("billing.archived")
        }}<NSwitch v-model:value="model.archived" :disabled="busy || plan.archived"
      /></label>
      <div>
        <NButton type="primary" attr-type="submit" :loading="busy" :disabled="!valid || busy">{{
          t("billing.save")
        }}</NButton
        ><NButton class="cancel" :disabled="busy" @click="$emit('close')">{{
          t("billing.cancel")
        }}</NButton>
      </div>
    </form></NCard
  >
</template>
<style scoped>
.plan-form {
  display: grid;
  gap: 16px;
  margin-top: 16px;
  max-width: 640px;
}
.plan-form label {
  display: grid;
  gap: 6px;
}
.plan-form .toggle {
  display: flex;
  gap: 12px;
  align-items: center;
}
.cancel {
  margin-left: 12px;
}
</style>
