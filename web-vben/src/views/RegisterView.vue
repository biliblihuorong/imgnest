<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { formatApiError } from "@/locales/errors";
import {
  NAlert,
  NSpin,
  NButton,
  NForm,
  NFormItem,
  NInput,
  NResult,
  NText,
  useMessage,
} from "naive-ui";
import { computed, onBeforeUnmount, ref, useTemplateRef } from "vue";
import { useRouter } from "vue-router";
import AuthCaptcha from "@/components/captcha/AuthCaptcha.vue";
import { register } from "@/api/auth";
import { useSiteStore } from "@/stores/site";

const { t } = useI18n();
const router = useRouter();
const site = useSiteStore();
const message = useMessage();

const captcha = useTemplateRef<InstanceType<typeof AuthCaptcha>>("captcha");
const captchaReady = ref(false);
const username = ref("");
const email = ref("");
const password = ref("");
const confirmPassword = ref("");
const submitting = ref(false);
let disposed = false;
let submission: AbortController | undefined;
const serverError = ref<unknown>(null);
const errorMessage = computed(() =>
  serverError.value ? formatApiError(serverError.value, "common.errors.unknown") : "",
);
const loadingSite = ref(false);
async function loadSite(): Promise<void> {
  loadingSite.value = true;
  await site.ensureLoaded();
  loadingSite.value = false;
}

void loadSite();
onBeforeUnmount(() => {
  disposed = true;
  submission?.abort();
  password.value = "";
  confirmPassword.value = "";
});

// 后端密码规则：12-72 字节；字节上限客户端预检，超限不发请求
const passwordBytes = computed(() => new TextEncoder().encode(password.value).length);

const canSubmit = computed(
  () =>
    site.loaded &&
    site.registerEnabled &&
    captchaReady.value &&
    username.value.trim().length >= 3 &&
    email.value.trim().length > 0 &&
    password.value.length >= 12 &&
    passwordBytes.value <= 72 &&
    password.value === confirmPassword.value,
);

async function onSubmit(): Promise<void> {
  if (submitting.value || !canSubmit.value) {
    return;
  }
  if (password.value !== confirmPassword.value) {
    return;
  }
  submitting.value = true;
  const request = new AbortController();
  submission = request;
  serverError.value = null;
  try {
    const captchaToken = captcha.value!.consume();
    await register(
      {
        username: username.value.trim(),
        email: email.value.trim(),
        password: password.value,
        ...(captchaToken === undefined ? {} : { captcha_token: captchaToken }),
      },
      request.signal,
    );
    if (disposed || request.signal.aborted) return;
    message.success(t("common.auth.registered"));
    await router.push("/login");
  } catch (error) {
    if (disposed || request.signal.aborted) return;
    serverError.value = error;
    await captcha.value?.reload();
  } finally {
    if (!disposed && submission === request) submitting.value = false;
  }
}
</script>

<template>
  <main class="register-page">
    <NSpin v-if="loadingSite" />
    <NAlert v-else-if="!site.loaded" type="error"
      >{{ t("common.errors.network") }}
      <NButton @click="loadSite">{{ t("common.retry") }}</NButton></NAlert
    >
    <section v-else-if="site.registerEnabled">
      <header class="auth-heading">
        <h1>{{ t("common.auth.create") }}</h1>
        <p>{{ t("common.auth.registerIntro", { site: site.siteName }) }}</p>
      </header>
      <NForm label-placement="top" @submit.prevent="onSubmit">
        <NFormItem :label="t('common.auth.username')">
          <NInput
            v-model:value="username"
            :placeholder="t('common.auth.usernameHint')"
            :disabled="submitting"
            :input-props="{ autocomplete: 'username', 'aria-label': t('common.auth.username') }"
          />
        </NFormItem>
        <NFormItem :label="t('common.auth.email')">
          <NInput
            v-model:value="email"
            :placeholder="t('common.auth.email')"
            :disabled="submitting"
            :input-props="{ autocomplete: 'email', 'aria-label': t('common.auth.email') }"
          />
        </NFormItem>
        <NFormItem :label="t('common.auth.password')">
          <NInput
            v-model:value="password"
            type="password"
            show-password-on="click"
            :placeholder="t('common.auth.passwordHint')"
            :disabled="submitting"
            :input-props="{ autocomplete: 'new-password', 'aria-label': t('common.auth.password') }"
          />
        </NFormItem>
        <NFormItem :label="t('common.auth.confirm')">
          <NInput
            v-model:value="confirmPassword"
            type="password"
            show-password-on="click"
            :placeholder="t('common.auth.confirmHint')"
            :disabled="submitting"
            :input-props="{ autocomplete: 'new-password', 'aria-label': t('common.auth.confirm') }"
          />
        </NFormItem>
        <AuthCaptcha ref="captcha" action="register" @ready="captchaReady = $event" />
        <p v-if="errorMessage" role="alert">
          <NText type="error">{{ errorMessage }}</NText>
        </p>
        <NButton
          attr-type="submit"
          type="primary"
          block
          :loading="submitting"
          :disabled="!canSubmit || submitting"
        >
          {{ t("common.register") }}
        </NButton>
      </NForm>
      <p class="register-footer">
        <RouterLink to="/login">{{ t("common.auth.backLogin") }}</RouterLink>
      </p>
    </section>
    <section v-else>
      <NResult
        status="403"
        :title="t('common.auth.closed')"
        :description="t('common.auth.closedDescription')"
      />
      <p class="register-footer">
        <RouterLink to="/login">{{ t("common.auth.backLogin") }}</RouterLink>
      </p>
    </section>
  </main>
</template>

<style scoped>
.register-page {
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
.register-footer {
  margin: 20px 0 0;
  text-align: center;
  color: hsl(var(--muted-foreground));
}
.register-footer a {
  color: hsl(var(--primary));
}
</style>
