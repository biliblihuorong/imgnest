<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NAlert, NButton, NCard, NCheckbox, NForm, NFormItem, NInput, NTag } from "naive-ui";
import { shallowRef } from "vue";
import CaptchaDraftTest from "@/components/captcha/CaptchaDraftTest.vue";
import { useCaptchaSettings } from "@/components/captcha/useCaptchaSettings";

const { t } = useI18n();
const {
  settings,
  siteKey,
  hostnameInput,
  secret,
  clearSecret,
  legacyAcknowledged,
  v1Acknowledged,
  loading,
  pending,
  busy,
  stale,
  errorMessage,
  notice,
  dirty,
  canSave,
  canTest,
  canActivate,
  draftConfig,
  reload,
  save,
  verify,
  activate,
  disable,
} = useCaptchaSettings();
const confirmingDisable = shallowRef(false);
async function confirmDisable(): Promise<void> {
  await disable();
  confirmingDisable.value = false;
}
</script>

<template>
  <NCard :title="t('captcha.admin.title')" class="captcha-settings">
    <template #header-extra>
      <NButton
        size="small"
        attr-type="button"
        data-action="reload"
        :disabled="busy"
        @click="reload(true)"
        >{{ t("captcha.admin.reload") }}</NButton
      >
    </template>
    <p class="description">{{ t("captcha.admin.intro") }}</p>
    <p v-if="loading" role="status" aria-live="polite">{{ t("captcha.configLoading") }}</p>
    <NAlert v-if="errorMessage" type="error" role="alert" class="notice">{{ errorMessage }}</NAlert>
    <NAlert v-if="stale" type="warning" class="notice">{{ t("captcha.admin.stale") }}</NAlert>
    <p v-if="notice" role="status" aria-live="polite">{{ t(`captcha.admin.notices.${notice}`) }}</p>
    <template v-if="settings">
      <div class="status-row" data-captcha-status>
        <NTag :type="settings.enabled ? 'success' : 'default'">{{
          t(settings.enabled ? "captcha.admin.enabled" : "captcha.admin.disabled")
        }}</NTag>
        <span>{{ t("captcha.admin.version", { version: settings.version }) }}</span>
      </div>
      <p v-if="settings.active" class="description">
        {{
          t("captcha.admin.activeConfig", {
            key: settings.active.site_key,
            hostnames: settings.active.hostnames.join(", "),
          })
        }}
      </p>
      <NAlert v-if="!settings.configuration_available" type="warning" class="notice">{{
        t("captcha.admin.configurationUnavailable")
      }}</NAlert>

      <section class="settings-section" :aria-label="t('captcha.admin.draftTitle')">
        <h3>{{ t("captcha.admin.draftTitle") }}</h3>
        <NForm label-placement="top" @submit.prevent="save">
          <NFormItem :label="t('captcha.admin.provider')"
            ><NInput
              :value="t('captcha.admin.turnstile')"
              readonly
              :input-props="{ 'aria-label': t('captcha.admin.provider') }"
          /></NFormItem>
          <NFormItem :label="t('captcha.admin.siteKey')"
            ><NInput
              v-model:value="siteKey"
              :disabled="busy || !settings.configuration_available"
              :input-props="{ 'aria-label': t('captcha.admin.siteKey'), autocomplete: 'off' }"
          /></NFormItem>
          <NFormItem :label="t('captcha.admin.hostnames')"
            ><NInput
              v-model:value="hostnameInput"
              type="textarea"
              :disabled="busy || !settings.configuration_available"
              :placeholder="t('captcha.admin.hostnameHint')"
              :input-props="{ 'aria-label': t('captcha.admin.hostnames') }"
          /></NFormItem>
          <NFormItem :label="t('captcha.admin.secret')"
            ><NInput
              v-model:value="secret"
              type="password"
              :disabled="busy || clearSecret || !settings.configuration_available"
              :placeholder="t('captcha.admin.secretHint')"
              :input-props="{
                'aria-label': t('captcha.admin.secret'),
                autocomplete: 'new-password',
              }"
          /></NFormItem>
          <p class="description">
            {{
              t(
                (settings.draft ?? settings.active)?.secret_configured
                  ? "captcha.admin.secretConfigured"
                  : "captcha.admin.secretMissing",
              )
            }}
            · {{ t("captcha.admin.secretHint") }}
          </p>
          <NCheckbox
            v-model:checked="clearSecret"
            data-clear-secret
            :aria-disabled="
              busy || settings.enabled || !!secret || !settings.configuration_available
            "
            :disabled="busy || settings.enabled || !!secret || !settings.configuration_available"
            >{{ t("captcha.admin.clearSecret") }}</NCheckbox
          >
          <p class="description">{{ t("captcha.admin.clearSecretHint") }}</p>
          <NButton
            type="primary"
            attr-type="submit"
            data-action="save"
            :loading="pending"
            :disabled="!canSave"
            >{{ t("captcha.admin.saveDraft") }}</NButton
          >
        </NForm>
      </section>

      <section class="settings-section" :aria-label="t('captcha.admin.testTitle')">
        <h3>{{ t("captcha.admin.testTitle") }}</h3>
        <p class="description">{{ t("captcha.admin.testHint") }}</p>
        <p v-if="dirty" class="description">{{ t("captcha.admin.unsaved") }}</p>
        <p class="description">
          {{ t("captcha.admin.privacy") }}
          <a
            href="https://www.cloudflare.com/turnstile-privacy-policy/"
            target="_blank"
            rel="noopener noreferrer"
            >{{ t("captcha.admin.privacyLink") }}</a
          >
        </p>
        <div class="test-grid">
          <CaptchaDraftTest
            v-for="action in ['login', 'register'] as const"
            :key="action"
            :action="action"
            :config="draftConfig"
            :disabled="!canTest"
            :tested="settings.draft?.tested_actions.includes(action) ?? false"
            @verify="verify"
          />
        </div>
      </section>

      <section class="settings-section" :aria-label="t('captcha.admin.activationTitle')">
        <h3>{{ t("captcha.admin.activationTitle") }}</h3>
        <div class="acknowledgements">
          <NCheckbox
            v-model:checked="legacyAcknowledged"
            data-ack="legacy"
            :disabled="busy || stale"
            >{{ t("captcha.admin.legacyRisk") }}</NCheckbox
          >
          <NCheckbox v-model:checked="v1Acknowledged" data-ack="v1" :disabled="busy || stale">{{
            t("captcha.admin.v1Risk")
          }}</NCheckbox>
        </div>
        <NButton
          type="primary"
          attr-type="button"
          data-action="activate"
          :loading="pending"
          :disabled="!canActivate"
          @click="activate"
          >{{ t("captcha.admin.activate") }}</NButton
        >
        <NButton
          v-if="settings.enabled && !confirmingDisable"
          attr-type="button"
          data-action="disable"
          :disabled="busy || stale"
          class="disable-button"
          @click="confirmingDisable = true"
          >{{ t("captcha.admin.disable") }}</NButton
        >
        <NAlert
          v-if="settings.enabled && confirmingDisable"
          type="warning"
          class="notice disable-confirmation"
        >
          <p>{{ t("captcha.admin.disableWarning") }}</p>
          <div class="confirm-actions">
            <NButton
              attr-type="button"
              data-action="cancel-disable"
              :disabled="busy"
              @click="confirmingDisable = false"
              >{{ t("captcha.admin.cancel") }}</NButton
            ><NButton
              attr-type="button"
              type="warning"
              data-action="confirm-disable"
              :loading="pending"
              :disabled="busy || stale"
              @click="confirmDisable"
              >{{ t("captcha.admin.confirmDisable") }}</NButton
            >
          </div>
        </NAlert>
      </section>
    </template>
  </NCard>
</template>

<style scoped>
.captcha-settings {
  margin-top: 24px;
  min-width: 0;
}
.description {
  color: hsl(var(--muted-foreground));
  overflow-wrap: anywhere;
}
.notice {
  margin: 12px 0;
}
.status-row,
.confirm-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
}
.settings-section {
  margin-top: 24px;
  padding-top: 20px;
  border-top: 1px solid hsl(var(--border));
}
.settings-section h3 {
  margin: 0 0 14px;
  font-size: 16px;
}
.test-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}
.acknowledgements {
  display: grid;
  gap: 14px;
  margin: 0 0 18px;
}
.disable-button {
  margin-inline-start: 12px;
}
.disable-confirmation {
  max-width: 700px;
}
@media (max-width: 768px) {
  .test-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
