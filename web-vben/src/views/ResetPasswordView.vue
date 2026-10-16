<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NAlert, NButton, NForm, NFormItem, NInput, NSpin, NText, useMessage } from "naive-ui";
import { computed, onBeforeUnmount, ref } from "vue";
import { useRouter } from "vue-router";
import { resetPassword } from "@/api/auth";
import EmailCodeField from "@/components/auth/EmailCodeField.vue";
import { formatApiError } from "@/locales/errors";
import { useSiteStore } from "@/stores/site";

const { t } = useI18n();
const router = useRouter();
const site = useSiteStore();
const message = useMessage();

const email = ref("");
const code = ref("");
const password = ref("");
const confirmPassword = ref("");
const submitting = ref(false);
const serverError = ref<unknown>(null);
const loadingSite = ref(true);
let disposed = false;

async function loadSite(): Promise<void> {
  loadingSite.value = true;
  await site.refresh();
  loadingSite.value = false;
}
void loadSite();
onBeforeUnmount(() => {
  disposed = true;
  password.value = "";
  confirmPassword.value = "";
});

const passwordBytes = computed(() => new TextEncoder().encode(password.value).length);
const mismatch = computed(
  () => confirmPassword.value.length > 0 && password.value !== confirmPassword.value,
);
const canSubmit = computed(
  () =>
    email.value.trim().length > 0 &&
    /^\d{6}$/.test(code.value.trim()) &&
    password.value.length >= 12 &&
    passwordBytes.value <= 72 &&
    password.value === confirmPassword.value,
);
const errorMessage = computed(() =>
  serverError.value ? formatApiError(serverError.value, "common.errors.unknown") : "",
);

async function onSubmit(): Promise<void> {
  if (submitting.value || !canSubmit.value) return;
  submitting.value = true;
  serverError.value = null;
  try {
    await resetPassword(email.value.trim(), code.value.trim(), password.value);
    if (disposed) return;
    message.success(t("common.auth.resetDone"));
    await router.push("/login");
  } catch (error) {
    if (!disposed) serverError.value = error;
  } finally {
    if (!disposed) submitting.value = false;
  }
}
</script>

<template>
  <main class="reset-page">
    <NSpin v-if="loadingSite" />
    <NAlert v-else-if="!site.loaded" type="error"
      >{{ t("common.errors.network") }}
      <NButton @click="loadSite">{{ t("common.retry") }}</NButton></NAlert
    >
    <section v-else-if="site.passwordReset">
      <header class="auth-heading">
        <h1>{{ t("common.auth.resetTitle") }}</h1>
        <p>{{ t("common.auth.resetIntro") }}</p>
      </header>
      <NForm label-placement="top" @submit.prevent="onSubmit">
        <NFormItem :label="t('common.auth.email')">
          <NInput
            v-model:value="email"
            :placeholder="t('common.auth.email')"
            :disabled="submitting"
            :input-props="{ autocomplete: 'email', 'aria-label': t('common.auth.email') }"
          />
        </NFormItem>
        <EmailCodeField
          v-model:code="code"
          :email="email"
          purpose="reset_password"
          :disabled="submitting"
        />
        <NFormItem :label="t('common.auth.newPassword')">
          <NInput
            v-model:value="password"
            type="password"
            show-password-on="click"
            :placeholder="t('common.auth.passwordHint')"
            :disabled="submitting"
            :input-props="{
              autocomplete: 'new-password',
              'aria-label': t('common.auth.newPassword'),
            }"
          />
        </NFormItem>
        <NFormItem
          :label="t('common.auth.confirm')"
          :validation-status="mismatch ? 'error' : undefined"
          :feedback="mismatch ? t('common.auth.mismatch') : undefined"
        >
          <NInput
            v-model:value="confirmPassword"
            type="password"
            show-password-on="click"
            :placeholder="t('common.auth.confirmHint')"
            :disabled="submitting"
            :input-props="{ autocomplete: 'new-password', 'aria-label': t('common.auth.confirm') }"
          />
        </NFormItem>
        <p v-if="errorMessage" role="alert">
          <NText type="error">{{ errorMessage }}</NText>
        </p>
        <NButton
          attr-type="submit"
          type="primary"
          block
          data-action="reset-password"
          :loading="submitting"
          :disabled="!canSubmit || submitting"
        >
          {{ t("common.auth.resetSubmit") }}
        </NButton>
      </NForm>
      <p class="reset-footer">
        <RouterLink to="/login">{{ t("common.auth.backLogin") }}</RouterLink>
      </p>
    </section>
    <section v-else>
      <NAlert type="info">{{ t("common.auth.resetUnavailable") }}</NAlert>
      <p class="reset-footer">
        <RouterLink to="/login">{{ t("common.auth.backLogin") }}</RouterLink>
      </p>
    </section>
  </main>
</template>

<style scoped>
.reset-page {
  width: 100%;
  max-width: 420px;
  margin: auto;
}
.auth-heading {
  margin-bottom: 28px;
}
.auth-heading h1 {
  margin: 0 0 8px;
  font-size: 26px;
  font-weight: 650;
  letter-spacing: -0.5px;
}
.auth-heading p {
  margin: 0;
  color: hsl(var(--muted-foreground));
}
.reset-footer {
  margin: 20px 0 0;
  text-align: center;
  color: hsl(var(--muted-foreground));
}
.reset-footer a {
  color: hsl(var(--primary));
}
</style>
