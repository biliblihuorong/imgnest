<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { useMediaQuery } from "@vueuse/core";
import {
  NAlert,
  NButton,
  NCard,
  NCheckbox,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NSelect,
  NSwitch,
  useMessage,
} from "naive-ui";
import { computed, onBeforeUnmount, onMounted, reactive, shallowRef } from "vue";
import DocHelp from "@/components/help/DocHelp.vue";
import { ApiError } from "@/api/client";
import {
  getMailSettings,
  saveMailSettings,
  sendTestMail,
  type MailEncryption,
  type MailSettings,
  type MailSettingsPayload,
} from "@/api/mail";
import { formatApiError } from "@/locales/errors";

const { t } = useI18n();
const message = useMessage();
const isMobile = useMediaQuery("(max-width: 640px)");

const saved = shallowRef<MailSettings | null>(null);
const loading = shallowRef(true);
const loadError = shallowRef<unknown>(null);
const saving = shallowRef(false);
const testing = shallowRef(false);
const testTo = shallowRef("");
const testError = shallowRef("");
const form = reactive({
  enabled: false,
  host: "",
  port: 465,
  encryption: "tls" as MailEncryption,
  username: "",
  password: "",
  clearPassword: false,
  fromAddress: "",
  fromName: "",
});

const defaultPorts: Record<MailEncryption, number> = { tls: 465, starttls: 587, none: 25 };
const encryptionOptions = computed(() => [
  { label: t("admin.settings.mail.encryptionTls"), value: "tls" },
  { label: t("admin.settings.mail.encryptionStarttls"), value: "starttls" },
  { label: t("admin.settings.mail.encryptionNone"), value: "none" },
]);
const busy = computed(() => loading.value || saving.value);
const disabled = computed(() => busy.value || !saved.value);
const dirty = computed(() => {
  const s = saved.value;
  if (!s) return false;
  return (
    form.enabled !== s.enabled ||
    form.host.trim() !== s.host ||
    form.port !== s.port ||
    form.encryption !== s.encryption ||
    form.username.trim() !== s.username ||
    form.fromAddress.trim() !== s.from_address ||
    form.fromName.trim() !== s.from_name ||
    !!form.password ||
    form.clearPassword
  );
});

function sync(settings: MailSettings): void {
  saved.value = settings;
  form.enabled = settings.enabled;
  form.host = settings.host;
  form.port = settings.port;
  form.encryption = settings.encryption;
  form.username = settings.username;
  form.password = "";
  form.clearPassword = false;
  form.fromAddress = settings.from_address;
  form.fromName = settings.from_name;
}

async function load(): Promise<void> {
  loading.value = true;
  loadError.value = null;
  try {
    sync(await getMailSettings());
  } catch (error) {
    loadError.value = error;
  } finally {
    loading.value = false;
  }
}

/** 换加密方式时，如果端口还是上一种方式的默认值，就一起换成新的默认端口。 */
function changeEncryption(next: MailEncryption): void {
  if (Object.values(defaultPorts).includes(form.port)) form.port = defaultPorts[next];
  form.encryption = next;
  if (next === "none") form.username = "";
}

async function save(): Promise<void> {
  if (disabled.value) return;
  const payload: MailSettingsPayload = {
    enabled: form.enabled,
    host: form.host.trim(),
    port: form.port,
    encryption: form.encryption,
    username: form.encryption === "none" ? "" : form.username.trim(),
    from_address: form.fromAddress.trim(),
    from_name: form.fromName.trim(),
  };
  if (form.clearPassword) payload.clear_password = true;
  else if (form.password) payload.password = form.password;
  saving.value = true;
  try {
    sync(await saveMailSettings(payload));
    message.success(() => t("admin.settings.mail.saved"));
  } catch (error) {
    message.error(() => formatApiError(error, "admin.errors.save"));
  } finally {
    saving.value = false;
  }
}

async function test(): Promise<void> {
  if (testing.value) return;
  testing.value = true;
  testError.value = "";
  try {
    await sendTestMail(testTo.value.trim());
    message.success(() => t("admin.settings.mail.testSent"));
  } catch (error) {
    const detail =
      error instanceof ApiError && error.code === 50006
        ? (error.data as { reason?: string; smtp_code?: number } | null)
        : null;
    testError.value = detail?.reason
      ? t(`admin.settings.mail.reasons.${detail.reason}`) +
        (detail.smtp_code ? ` (SMTP ${detail.smtp_code})` : "")
      : formatApiError(error, "common.errors.unknown");
  } finally {
    testing.value = false;
  }
}

onMounted(() => void load());
onBeforeUnmount(() => {
  form.password = "";
});
</script>

<template>
  <NCard :bordered="false" class="mail-settings">
    <template #header>{{ t("admin.settings.mail.title") }}<DocHelp topic="mail" /></template>
    <NAlert v-if="loadError" type="error" class="notice">
      {{ formatApiError(loadError, "admin.errors.settingsLoad") }}
      <NButton size="tiny" quaternary type="primary" :disabled="loading" @click="load">{{
        t("admin.common.retry")
      }}</NButton>
    </NAlert>
    <NAlert v-if="saved && !saved.configuration_available" type="warning" class="notice">{{
      t("admin.settings.mail.noMasterKey")
    }}</NAlert>
    <NForm
      class="mail-form"
      :label-placement="isMobile ? 'top' : 'left'"
      :label-width="isMobile ? undefined : 150"
      @submit.prevent="save"
    >
      <NFormItem :label="t('admin.settings.mail.enabled')">
        <div class="field">
          <NSwitch v-model:value="form.enabled" data-mail-enabled :disabled="disabled" />
          <p class="field-hint">{{ t("admin.settings.mail.enabledHint") }}</p>
        </div>
      </NFormItem>
      <NFormItem :label="t('admin.settings.mail.host')">
        <NInput
          v-model:value="form.host"
          :placeholder="t('admin.settings.mail.hostPlaceholder')"
          :disabled="disabled"
          :input-props="{ 'aria-label': t('admin.settings.mail.host'), autocomplete: 'off' }"
        />
      </NFormItem>
      <NFormItem :label="t('admin.settings.mail.encryption')">
        <NSelect
          :value="form.encryption"
          :options="encryptionOptions"
          :disabled="disabled"
          @update:value="changeEncryption"
        />
      </NFormItem>
      <NFormItem :label="t('admin.settings.mail.port')">
        <NInputNumber
          v-model:value="form.port"
          class="field-control"
          :min="1"
          :max="65535"
          :disabled="disabled"
        />
      </NFormItem>
      <NFormItem v-if="form.encryption !== 'none'" :label="t('admin.settings.mail.username')">
        <div class="field">
          <NInput
            v-model:value="form.username"
            :disabled="disabled"
            :input-props="{ 'aria-label': t('admin.settings.mail.username'), autocomplete: 'off' }"
          />
          <p class="field-hint">{{ t("admin.settings.mail.usernameHint") }}</p>
        </div>
      </NFormItem>
      <NFormItem v-if="form.encryption !== 'none'" :label="t('admin.settings.mail.password')">
        <div class="field">
          <NInput
            v-model:value="form.password"
            type="password"
            show-password-on="click"
            :disabled="disabled || form.clearPassword || !saved?.configuration_available"
            :placeholder="saved?.password_configured ? t('admin.settings.mail.passwordKeep') : ''"
            :input-props="{
              'aria-label': t('admin.settings.mail.password'),
              autocomplete: 'new-password',
            }"
          />
          <p class="field-hint">{{ t("admin.settings.mail.passwordHint") }}</p>
          <NCheckbox
            v-if="saved?.password_configured"
            v-model:checked="form.clearPassword"
            :disabled="disabled || !!form.password"
            >{{ t("admin.settings.mail.clearPassword") }}</NCheckbox
          >
        </div>
      </NFormItem>
      <NFormItem :label="t('admin.settings.mail.fromAddress')">
        <NInput
          v-model:value="form.fromAddress"
          :placeholder="t('admin.settings.mail.fromAddressPlaceholder')"
          :disabled="disabled"
          :input-props="{ 'aria-label': t('admin.settings.mail.fromAddress') }"
        />
      </NFormItem>
      <NFormItem :label="t('admin.settings.mail.fromName')">
        <div class="field">
          <NInput
            v-model:value="form.fromName"
            :disabled="disabled"
            :input-props="{ 'aria-label': t('admin.settings.mail.fromName') }"
          />
          <p class="field-hint">{{ t("admin.settings.mail.fromNameHint") }}</p>
        </div>
      </NFormItem>
      <div class="actions">
        <NButton
          type="primary"
          attr-type="submit"
          data-action="save-mail"
          :loading="saving"
          :disabled="disabled"
          >{{ t("admin.settings.mail.save") }}</NButton
        >
      </div>
    </NForm>

    <section class="test-section">
      <h3>{{ t("admin.settings.mail.testTitle") }}</h3>
      <p v-if="dirty" class="field-hint">{{ t("admin.settings.mail.testNeedsSave") }}</p>
      <div class="test-row">
        <NInput
          v-model:value="testTo"
          :placeholder="t('admin.settings.mail.testToPlaceholder')"
          :disabled="testing || !saved?.enabled"
          :input-props="{ 'aria-label': t('admin.settings.mail.testTo'), type: 'email' }"
        />
        <NButton
          data-action="test-mail"
          :loading="testing"
          :disabled="testing || !saved?.enabled || dirty"
          @click="test"
          >{{ t("admin.settings.mail.testSend") }}</NButton
        >
      </div>
      <NAlert v-if="testError" type="error" role="alert" class="notice" data-mail-test-error>{{
        testError
      }}</NAlert>
    </section>
  </NCard>
</template>

<style scoped>
.mail-form {
  max-width: 640px;
}
.field,
.field-control {
  width: 100%;
}
.field-hint {
  margin: 4px 0 0;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}
.notice {
  margin: 12px 0;
}
.actions {
  max-width: 640px;
  display: flex;
  justify-content: flex-end;
}
.test-section {
  max-width: 640px;
  margin-top: 24px;
  padding-top: 20px;
  border-top: 1px solid hsl(var(--border));
}
.test-section h3 {
  margin: 0 0 8px;
  font-size: 15px;
}
.test-row {
  display: flex;
  gap: 12px;
  margin-top: 8px;
}
@media (max-width: 640px) {
  .test-row {
    flex-direction: column;
  }
}
</style>
