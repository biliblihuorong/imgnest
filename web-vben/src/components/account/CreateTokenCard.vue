<script setup lang="ts">
import { useI18n } from "@vben/locales";
import {
  NAlert,
  NButton,
  NCard,
  NDatePicker,
  NForm,
  NFormItem,
  NInput,
  NModal,
  NSpace,
  NTag,
  useMessage,
  type FormInst,
  type FormItemRule,
  type FormRules,
} from "naive-ui";
import { reactive, ref, shallowRef, watch } from "vue";
import { createToken, type IssuedToken, type CreateTokenPayload } from "@/api/tokens";
import { formatApiError } from "@/locales/errors";
import { useAccountScope } from "./useAccountScope";

const emit = defineEmits<{ saved: [] }>();
const { t, locale } = useI18n();
const message = useMessage();
const formRef = ref<FormInst | null>(null);
const creating = shallowRef(false);
const copying = shallowRef(false);
const model = reactive<{ name: string; expires_at: number | null }>({ name: "", expires_at: null });
const issued = shallowRef<IssuedToken | null>(null);
const scope = useAccountScope(() => {
  issued.value = null;
  model.name = "";
  model.expires_at = null;
});
const validated = new Set<string>();
const rules: FormRules = {
  name: [
    {
      key: "name",
      required: true,
      trigger: ["blur", "input"],
      validator: (_rule: FormItemRule, value: string) => {
        validated.add("name");
        return value.trim() ? true : new Error(t("account.nameRequired"));
      },
    },
  ],
  expires_at: [
    {
      key: "expires_at",
      trigger: ["blur", "change"],
      validator: (_rule: FormItemRule, value: number | null) => {
        validated.add("expires_at");
        return value === null ||
          (Number.isFinite(value) && value > Date.now() && !Number.isNaN(new Date(value).getTime()))
          ? true
          : new Error(t("account.expiryInvalid"));
      },
    },
  ],
};
watch(locale, () => {
  if (validated.size && scope.available.value) {
    void formRef.value
      ?.validate(undefined, (rule) => validated.has(rule.key as string))
      .catch(() => {});
  }
});
async function submitCreate(): Promise<void> {
  if (creating.value || issued.value || !scope.available.value) return;
  creating.value = true;
  const version = scope.capture();
  try {
    try {
      await formRef.value?.validate();
    } catch {
      return;
    }
    if (!scope.isCurrent(version)) return;
    const payload: CreateTokenPayload = { name: model.name.trim() };
    if (model.expires_at !== null) payload.expires_at = new Date(model.expires_at).toISOString();
    const result = await createToken(payload);
    if (!scope.isCurrent(version)) return;
    issued.value = result;
    model.name = "";
    model.expires_at = null;
    validated.clear();
    formRef.value?.restoreValidation();
  } catch (error) {
    if (scope.isCurrent(version)) message.error(() => formatApiError(error, "account.createError"));
  } finally {
    creating.value = false;
  }
}
async function copyIssuedToken(): Promise<void> {
  if (!issued.value || copying.value || !scope.available.value) return;
  copying.value = true;
  const version = scope.capture();
  try {
    await navigator.clipboard.writeText(issued.value.token);
    if (scope.isCurrent(version) && issued.value) message.success(() => t("account.copied"));
  } catch {
    if (scope.isCurrent(version) && issued.value) message.error(() => t("account.copyError"));
  } finally {
    copying.value = false;
  }
}
function closeIssuedModal(): void {
  if (!issued.value) return;
  issued.value = null;
  emit("saved");
}
</script>

<template>
  <NCard :title="t('account.createTitle')" :bordered="false">
    <NForm
      ref="formRef"
      class="tokens-view__form"
      :model="model"
      :rules="rules"
      label-placement="top"
      @submit.prevent="submitCreate"
    >
      <div class="token-create__row">
        <NFormItem :label="t('account.name')" path="name" class="token-create__name">
          <NInput
            v-model:value="model.name"
            :placeholder="t('account.namePlaceholder')"
            :disabled="creating || !scope.available.value"
          />
        </NFormItem>
        <NFormItem :label="t('account.expiresAt')" path="expires_at" class="token-create__expiry">
          <NDatePicker
            v-model:value="model.expires_at"
            type="datetime"
            clearable
            :placeholder="t('account.expiryPlaceholder')"
            :disabled="creating || !scope.available.value"
            style="width: 100%"
          />
        </NFormItem>
        <NButton
          class="token-create__submit"
          attr-type="submit"
          type="primary"
          :loading="creating"
          :disabled="!scope.available.value || issued !== null"
          >{{ t("account.create") }}</NButton
        >
      </div>
      <p class="token-create__hint">
        <NTag size="small" :bordered="false" type="success">api</NTag
        ><span>{{ t("account.creationHint") }}</span>
      </p>
    </NForm>
  </NCard>
  <NModal
    :show="issued !== null"
    preset="card"
    :title="t('account.issuedTitle')"
    class="token-create__modal"
    :mask-closable="false"
    :closable="false"
    :close-on-esc="false"
  >
    <NAlert type="warning" :title="t('account.saveNow')">{{ t("account.saveWarning") }}</NAlert>
    <p class="token-create__label">
      {{ t("account.issuedLabel", { name: issued?.info.name ?? "" }) }}
    </p>
    <pre class="token-create__secret">{{ issued?.token }}</pre>
    <template #footer
      ><NSpace
        ><NButton :loading="copying" @click="copyIssuedToken">{{ t("account.copy") }}</NButton
        ><NButton type="primary" @click="closeIssuedModal">{{
          t("account.closeSaved")
        }}</NButton></NSpace
      ></template
    >
  </NModal>
</template>
<style scoped>
.token-create__row {
  display: flex;
  flex-wrap: wrap;
  gap: 0 16px;
  align-items: flex-start;
}
.token-create__name {
  flex: 1 1 220px;
  min-width: 0;
}
.token-create__expiry {
  flex: 1 1 260px;
  min-width: 0;
}
.token-create__submit {
  margin-top: 29px;
}
.token-create__hint {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin: 0;
  font-size: 13px;
  color: hsl(var(--muted-foreground));
  line-height: 1.7;
}
.token-create__hint :deep(.n-tag) {
  flex-shrink: 0;
}
.token-create__modal {
  width: min(640px, calc(100vw - 32px));
}
.token-create__label {
  margin: 16px 0 8px;
  font-weight: 600;
}
.token-create__secret {
  margin: 0 0 16px;
  padding: 12px;
  font-family: Consolas, Menlo, monospace;
  background: hsl(var(--muted));
  border-radius: 4px;
  white-space: pre-wrap;
  word-break: break-all;
  user-select: all;
}
@media (max-width: 640px) {
  .token-create__name,
  .token-create__expiry {
    flex-basis: 100%;
  }
  .token-create__submit {
    margin-top: 0;
    margin-bottom: 20px;
    width: 100%;
  }
}
</style>
