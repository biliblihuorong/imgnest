<script setup lang="ts">
import { NButton, NForm, NFormItem, NInput, NText } from "naive-ui";
import { computed, onBeforeUnmount, ref, useTemplateRef } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "@vben/locales";
import { formatApiError } from "@/locales/errors";
import { landingPath, safeRedirect } from "@/router/landing";
import AuthCaptcha from "@/components/captcha/AuthCaptcha.vue";
import { useAuthStore } from "@/stores/auth";
import { useSiteStore } from "@/stores/site";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const site = useSiteStore();

const captcha = useTemplateRef<InstanceType<typeof AuthCaptcha>>("captcha");
const captchaReady = ref(false);
const email = ref("");
const password = ref("");
const submitting = ref(false);
let disposed = false;
let submission: AbortController | undefined;
const serverError = ref<unknown>(null);
const errorMessage = computed(() =>
  serverError.value ? formatApiError(serverError.value, "common.errors.network") : "",
);

const ssoErrorKeys = new Set(["not_linked", "email_required", "disabled", "unavailable", "failed"]);
const ssoError = computed(() => {
  const code = route.query.sso_error;
  if (typeof code !== "string") return "";
  return t(`common.auth.ssoErrors.${ssoErrorKeys.has(code) ? code : "failed"}`);
});
// 只接受同源路径，防止配置错误的扩展把用户带去别的站点。
const ssoProviders = computed(() =>
  site.loginProviders.filter((p) => p.start_url.startsWith("/") && !p.start_url.startsWith("//")),
);

const canSubmit = computed(
  () => captchaReady.value && email.value.trim().length > 0 && password.value.length > 0,
);

// 登录/注册入口每次都强制刷新站点配置，后台切换注册开关后无需整页刷新。
void site.refresh();
onBeforeUnmount(() => {
  disposed = true;
  submission?.abort();
  password.value = "";
});

async function onSubmit(): Promise<void> {
  if (submitting.value || !canSubmit.value) {
    return;
  }
  submitting.value = true;
  const request = new AbortController();
  submission = request;
  const redirect = route.query.redirect;
  serverError.value = null;
  try {
    const captchaToken = captcha.value!.consume();
    const login = auth.login(email.value.trim(), password.value, captchaToken, request.signal);
    const generation = auth.sessionGeneration;
    const user = await login;
    if (disposed || request.signal.aborted || auth.sessionGeneration !== generation) return;
    await router.push(safeRedirect(redirect, landingPath(user.role), router));
  } catch (error) {
    if (
      disposed ||
      request.signal.aborted ||
      (error instanceof Error && error.name === "AbortError")
    )
      return;
    serverError.value = error;
    await captcha.value?.reload();
  } finally {
    if (!disposed && submission === request) submitting.value = false;
  }
}
</script>

<template>
  <main class="login-page">
    <header class="auth-heading">
      <h1>{{ t("common.auth.heading", { site: site.siteName }) }}</h1>
      <p>{{ t("common.auth.welcome") }}</p>
    </header>
    <NForm label-placement="top" @submit.prevent="onSubmit">
      <NFormItem :label="t('common.auth.email')">
        <NInput
          v-model:value="email"
          :placeholder="t('common.auth.email')"
          :input-props="{ autocomplete: 'email', 'aria-label': t('common.auth.email') }"
          :disabled="submitting"
          size="large"
        />
      </NFormItem>
      <NFormItem :label="t('common.auth.password')">
        <NInput
          v-model:value="password"
          type="password"
          show-password-on="click"
          :placeholder="t('common.auth.password')"
          :input-props="{
            autocomplete: 'current-password',
            'aria-label': t('common.auth.password'),
          }"
          :disabled="submitting"
          size="large"
        />
      </NFormItem>
      <AuthCaptcha ref="captcha" action="login" @ready="captchaReady = $event" />
      <p v-if="errorMessage || ssoError" role="alert">
        <NText type="error">{{ errorMessage || ssoError }}</NText>
      </p>
      <NButton
        attr-type="submit"
        type="primary"
        size="large"
        block
        :loading="submitting"
        :disabled="!canSubmit || submitting"
      >
        {{ t("common.login") }}
      </NButton>
    </NForm>
    <template v-if="ssoProviders.length > 0">
      <p class="sso-divider">{{ t("common.auth.ssoDivider") }}</p>
      <div class="sso-providers">
        <NButton
          v-for="provider in ssoProviders"
          :key="provider.start_url"
          tag="a"
          :href="provider.start_url"
          size="large"
          block
          secondary
        >
          {{ t("common.auth.ssoContinue", { name: provider.name }) }}
        </NButton>
      </div>
    </template>
    <p v-if="site.registerEnabled" class="login-footer">
      {{ t("common.auth.toRegisterHint") }}
      <RouterLink to="/register">{{ t("common.auth.toRegister") }}</RouterLink>
    </p>
  </main>
</template>

<style scoped>
.login-page {
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
.sso-divider {
  margin: 20px 0 12px;
  text-align: center;
  color: hsl(var(--muted-foreground));
}
.sso-providers {
  display: grid;
  gap: 10px;
}
.login-footer {
  margin: 20px 0 0;
  text-align: center;
  color: hsl(var(--muted-foreground));
}
.login-footer a {
  color: hsl(var(--primary));
}
</style>
