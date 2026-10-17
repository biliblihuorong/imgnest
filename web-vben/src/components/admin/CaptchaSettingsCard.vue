<script setup lang="ts">
import { useI18n } from "@vben/locales";
import {
  NAlert,
  NButton,
  NCard,
  NCollapse,
  NCollapseItem,
  NForm,
  NFormItem,
  NInput,
  NSwitch,
  NTag,
} from "naive-ui";
import { computed, onBeforeUnmount, onMounted, shallowRef } from "vue";
import DocHelp from "@/components/help/DocHelp.vue";
import {
  configureCaptcha,
  getCaptchaSettings,
  type CaptchaAdminView,
  type CaptchaConfigurePayload,
  type PublicCaptchaConfig,
} from "@/api/captcha";
import { ApiError } from "@/api/client";
import CaptchaChallenge from "@/components/captcha/CaptchaChallenge.vue";
import { formatApiError } from "@/locales/errors";

const { t } = useI18n();
const settings = shallowRef<CaptchaAdminView | null>(null);
const enabled = shallowRef(false);
const siteKey = shallowRef("");
const secret = shallowRef("");
const hostnameInput = shallowRef("");
const token = shallowRef<string | null>(null);
const resetKey = shallowRef(0);
const loading = shallowRef(true);
const pending = shallowRef(false);
const stale = shallowRef(false);
const serverError = shallowRef<unknown>(null);
const notice = shallowRef<"saved" | "activated" | "disabled" | null>(null);
let disposed = false;

const busy = computed(() => loading.value || pending.value);
const available = computed(() => !!settings.value?.configuration_available);
/** 服务器沿用的密钥：候选优先，其次当前生效的配置。 */
const candidate = computed(() => {
  const view = settings.value;
  if (!view) return null;
  return view.draft?.secret_configured ? view.draft : (view.active ?? view.draft);
});
const secretConfigured = computed(() => !!candidate.value?.secret_configured);
const hostnames = computed(() =>
  hostnameInput.value
    .split(/[\n,]/)
    .map((name) => name.trim().toLowerCase())
    .filter(Boolean),
);
const keyValid = computed(() => /^[a-zA-Z0-9_-]{1,256}$/.test(siteKey.value.trim()));
const configComplete = computed(
  () => keyValid.value && hostnames.value.length > 0 && (!!secret.value || secretConfigured.value),
);
/** 开启时用刚填的 Site Key 渲染验证码，证明 Key、密钥和域名能配合工作。 */
const challengeConfig = computed<PublicCaptchaConfig | null>(() =>
  enabled.value && keyValid.value && available.value
    ? {
        enabled: true,
        provider: "turnstile",
        site_key: siteKey.value.trim(),
        version: settings.value?.version ?? 0,
      }
    : null,
);
const canSave = computed(() => {
  if (!settings.value || busy.value || stale.value) return false;
  if (enabled.value) return available.value && configComplete.value && !!token.value;
  // 关闭：没填 Key 只是关掉保护；填了 Key 就一并保存配置以便下次开启。
  if (!siteKey.value.trim()) return settings.value.enabled;
  return available.value && configComplete.value;
});
const errorMessage = computed(() => (serverError.value ? formatApiError(serverError.value) : ""));

function defaultHostname(): string {
  return typeof window === "undefined" ? "" : window.location.hostname;
}

function syncForm(view: CaptchaAdminView): void {
  const current = view.enabled ? view.active : (view.draft ?? view.active);
  enabled.value = view.enabled;
  siteKey.value = current?.site_key ?? "";
  hostnameInput.value = (current?.hostnames.length ? current.hostnames : [defaultHostname()])
    .filter(Boolean)
    .join("\n");
  secret.value = "";
  resetChallenge();
}

function resetChallenge(): void {
  token.value = null;
  resetKey.value++;
}

async function reload(): Promise<void> {
  if (pending.value) return;
  loading.value = true;
  serverError.value = null;
  notice.value = null;
  try {
    const next = await getCaptchaSettings();
    if (disposed) return;
    const keepEdits = settings.value !== null && stale.value;
    settings.value = next;
    stale.value = false;
    if (!keepEdits) syncForm(next);
  } catch (error) {
    if (!disposed) serverError.value = error;
  } finally {
    if (!disposed) loading.value = false;
  }
}

async function save(): Promise<void> {
  if (!canSave.value || !settings.value) return;
  const payload: CaptchaConfigurePayload = {
    expected_version: settings.value.version,
    enabled: enabled.value,
  };
  if (siteKey.value.trim()) {
    payload.site_key = siteKey.value.trim();
    payload.hostnames = hostnames.value;
    if (secret.value) payload.secret = secret.value;
  }
  if (enabled.value && token.value) payload.captcha_token = token.value;
  pending.value = true;
  serverError.value = null;
  notice.value = null;
  // 令牌只能用一次，无论结果如何都重新加载验证码。
  resetChallenge();
  try {
    const next = await configureCaptcha(payload);
    if (disposed) return;
    settings.value = next;
    syncForm(next);
    notice.value = next.enabled ? "activated" : payload.site_key ? "saved" : "disabled";
  } catch (error) {
    if (disposed) return;
    serverError.value = error;
    if (error instanceof ApiError && error.code === 30011) stale.value = true;
  } finally {
    if (!disposed) pending.value = false;
  }
}

onMounted(() => void reload());
onBeforeUnmount(() => {
  disposed = true;
  secret.value = "";
  token.value = null;
});
</script>

<template>
  <NCard class="captcha-settings">
    <template #header>{{ t("captcha.admin.title") }}<DocHelp topic="signInProtection" /></template>
    <template #header-extra>
      <NTag v-if="settings" :type="settings.enabled ? 'success' : 'default'" data-captcha-status>{{
        t(settings.enabled ? "captcha.admin.enabled" : "captcha.admin.disabled")
      }}</NTag>
    </template>
    <p class="description">{{ t("captcha.admin.intro") }}</p>
    <p v-if="loading" role="status" aria-live="polite">{{ t("captcha.configLoading") }}</p>
    <NAlert v-if="errorMessage" type="error" role="alert" class="notice">{{ errorMessage }}</NAlert>
    <NAlert v-if="stale" type="warning" class="notice">
      {{ t("captcha.admin.stale") }}
      <NButton size="tiny" quaternary type="primary" data-action="reload" @click="reload">{{
        t("captcha.admin.reload")
      }}</NButton>
    </NAlert>
    <NAlert v-if="notice" type="success" role="status" class="notice">{{
      t(`captcha.admin.notices.${notice}`)
    }}</NAlert>
    <template v-if="settings">
      <NAlert v-if="!available" type="warning" class="notice">{{
        t("captcha.admin.configurationUnavailable")
      }}</NAlert>
      <NForm label-placement="top" class="captcha-form" @submit.prevent="save">
        <NFormItem :label="t('captcha.admin.enable')">
          <div class="field">
            <NSwitch
              v-model:value="enabled"
              data-captcha-enable
              :disabled="busy || (!available && !settings.enabled)"
            />
            <p class="description">{{ t("captcha.admin.enableHint") }}</p>
          </div>
        </NFormItem>
        <NFormItem :label="t('captcha.admin.siteKey')">
          <NInput
            v-model:value="siteKey"
            :disabled="busy || !available"
            :input-props="{ 'aria-label': t('captcha.admin.siteKey'), autocomplete: 'off' }"
            @update:value="resetChallenge"
          />
        </NFormItem>
        <NFormItem :label="t('captcha.admin.secret')">
          <NInput
            v-model:value="secret"
            type="password"
            show-password-on="click"
            :disabled="busy || !available"
            :placeholder="
              secretConfigured ? t('captcha.admin.secretKeep') : t('captcha.admin.secretRequired')
            "
            :input-props="{ 'aria-label': t('captcha.admin.secret'), autocomplete: 'new-password' }"
          />
        </NFormItem>
        <NCollapse class="advanced">
          <NCollapseItem :title="t('captcha.admin.advanced')" name="advanced">
            <NFormItem :label="t('captcha.admin.hostnames')">
              <div class="field">
                <NInput
                  v-model:value="hostnameInput"
                  type="textarea"
                  :autosize="{ minRows: 2, maxRows: 6 }"
                  :disabled="busy || !available"
                  :input-props="{ 'aria-label': t('captcha.admin.hostnames') }"
                />
                <p class="description">{{ t("captcha.admin.hostnamesHint") }}</p>
              </div>
            </NFormItem>
          </NCollapseItem>
        </NCollapse>
        <section v-if="challengeConfig" class="verify" data-captcha-verify>
          <h3>{{ t("captcha.admin.verifyTitle") }}</h3>
          <p class="description">{{ t("captcha.admin.verifyHint") }}</p>
          <CaptchaChallenge
            :key="challengeConfig.site_key"
            :config="challengeConfig"
            action="login"
            :reset-key="resetKey"
            size="compact"
            @token="token = $event"
          />
          <p class="description">
            {{ t("captcha.admin.privacy") }}
            <a
              href="https://www.cloudflare.com/turnstile-privacy-policy/"
              target="_blank"
              rel="noopener noreferrer"
              >{{ t("captcha.admin.privacyLink") }}</a
            >
          </p>
        </section>
        <NButton
          type="primary"
          attr-type="submit"
          data-action="save-captcha"
          :loading="pending"
          :disabled="!canSave"
          >{{ t("captcha.admin.save") }}</NButton
        >
      </NForm>
    </template>
  </NCard>
</template>

<style scoped>
.captcha-settings {
  margin-top: 20px;
  min-width: 0;
}
.captcha-form {
  max-width: 640px;
}
.field {
  width: 100%;
}
.description {
  margin: 4px 0 0;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
  overflow-wrap: anywhere;
}
.notice {
  margin: 12px 0;
}
.advanced {
  margin-bottom: 16px;
}
.verify {
  margin: 8px 0 20px;
  padding: 16px;
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
}
.verify h3 {
  margin: 0 0 4px;
  font-size: 15px;
}
</style>
