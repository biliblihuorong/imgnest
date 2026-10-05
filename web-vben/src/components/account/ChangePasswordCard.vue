<script setup lang="ts">
import { useI18n } from "@vben/locales";
import {
  NAlert,
  NButton,
  NForm,
  NFormItem,
  NInput,
  useMessage,
  type FormInst,
  type FormItemRule,
  type FormRules,
} from "naive-ui";
import { computed, reactive, ref, shallowRef, watch } from "vue";
import { useRouter } from "vue-router";
import { changePassword } from "@/api/auth";
import { formatApiError } from "@/locales/errors";
import { useAuthStore } from "@/stores/auth";
import { useAccountScope } from "./useAccountScope";

const { t, locale } = useI18n();
const message = useMessage();
const router = useRouter();
const auth = useAuthStore();
const formRef = ref<FormInst | null>(null);
const model = reactive({ current: "", next: "", confirm: "" });
const submitting = shallowRef(false);
const serverError = shallowRef<unknown>(null);
const errorText = computed(() => {
  void locale.value;
  return serverError.value ? formatApiError(serverError.value, "account.password.error") : null;
});
function clearSecrets() {
  model.current = "";
  model.next = "";
  model.confirm = "";
  serverError.value = null;
}
const scope = useAccountScope(clearSecrets);
const byteLength = (value: string) => new TextEncoder().encode(value).length;
const validated = new Set<string>();
function validationError(field: string, key?: string): true | Error {
  validated.add(field);
  return key ? new Error(t(`account.password.${key}`)) : true;
}
const rules: FormRules = {
  current: [
    {
      required: true,
      trigger: ["blur", "input"],
      validator: (_rule: FormItemRule, value: string) =>
        validationError(
          "current",
          !value ? "currentRequired" : byteLength(value) > 72 ? "currentTooLong" : undefined,
        ),
    },
  ],
  next: [
    {
      required: true,
      trigger: ["blur", "input"],
      validator: (_rule: FormItemRule, value: string) =>
        validationError(
          "next",
          !value
            ? "nextRequired"
            : byteLength(value) < 12
              ? "tooShort"
              : byteLength(value) > 72
                ? "tooLong"
                : undefined,
        ),
    },
  ],
  confirm: [
    {
      required: true,
      trigger: ["blur", "input"],
      validator: (_rule: FormItemRule, value: string) =>
        validationError(
          "confirm",
          !value ? "confirmRequired" : value !== model.next ? "mismatch" : undefined,
        ),
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
// Keys let locale changes refresh only fields already validated by the user.
for (const [key, fieldRules] of Object.entries(rules)) {
  for (const rule of Array.isArray(fieldRules) ? fieldRules : [fieldRules]) rule.key = key;
}

async function onSubmit(): Promise<void> {
  if (submitting.value || !scope.available.value) return;
  // Lock before asynchronous validation so repeated Enter/clicks submit only once.
  submitting.value = true;
  serverError.value = null;
  const version = scope.capture();
  try {
    try {
      await formRef.value?.validate();
    } catch {
      return;
    }
    if (!scope.isCurrent(version)) return;
    const requestedSession = auth.token;
    await changePassword(model.current, model.next);
    // Navigation does not undo server revocation; never clear a newer session.
    if (auth.token !== requestedSession) return;
    const showSuccess = scope.isCurrent(version);
    clearSecrets();
    if (showSuccess) message.success(() => t("account.password.success"));
    // The backend already revokes every token. Do not issue a second logout request.
    auth.clear();
    await router.push("/login");
  } catch (error) {
    if (scope.isCurrent(version)) serverError.value = error;
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <NForm
    ref="formRef"
    class="change-password"
    :model="model"
    :rules="rules"
    label-placement="top"
    @submit.prevent="onSubmit"
  >
    <NFormItem :label="t('account.password.current')" path="current">
      <NInput
        v-model:value="model.current"
        type="password"
        show-password-on="click"
        :placeholder="t('account.password.current')"
        :input-props="{ autocomplete: 'current-password' }"
        :disabled="submitting || !scope.available.value"
      />
    </NFormItem>
    <NFormItem :label="t('account.password.next')" path="next">
      <NInput
        v-model:value="model.next"
        type="password"
        show-password-on="click"
        :placeholder="t('account.password.nextPlaceholder')"
        :input-props="{ autocomplete: 'new-password' }"
        :disabled="submitting || !scope.available.value"
      />
    </NFormItem>
    <NFormItem :label="t('account.password.confirm')" path="confirm">
      <NInput
        v-model:value="model.confirm"
        type="password"
        show-password-on="click"
        :placeholder="t('account.password.confirmPlaceholder')"
        :input-props="{ autocomplete: 'new-password' }"
        :disabled="submitting || !scope.available.value"
      />
    </NFormItem>
    <NAlert v-if="errorText" type="error" class="change-password__error">{{ errorText }}</NAlert>
    <NButton
      attr-type="submit"
      type="primary"
      :loading="submitting"
      :disabled="!scope.available.value"
      >{{ t("account.password.submit") }}</NButton
    >
  </NForm>
</template>

<style scoped>
.change-password {
  max-width: 520px;
}
.change-password__error {
  margin-bottom: 16px;
}
</style>
