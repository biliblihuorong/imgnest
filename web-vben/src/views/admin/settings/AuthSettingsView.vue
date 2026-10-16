<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NAlert } from "naive-ui";
import { onMounted, shallowRef } from "vue";
import CaptchaSettingsCard from "@/components/admin/CaptchaSettingsCard.vue";
import { getMailSettings } from "@/api/mail";
import SettingsFieldsForm from "./SettingsFieldsForm.vue";
import { DEFAULT_GROUP, EMAIL_VERIFICATION, REGISTRATION } from "./fields";

const { t } = useI18n();
const fields = [REGISTRATION, DEFAULT_GROUP, EMAIL_VERIFICATION];
/** null 表示还没读到；读失败时不提示，避免误报。 */
const mailEnabled = shallowRef<boolean | null>(null);

onMounted(async () => {
  try {
    mailEnabled.value = (await getMailSettings()).enabled;
  } catch {
    mailEnabled.value = null;
  }
});
</script>

<template>
  <SettingsFieldsForm :fields="fields">
    <template #after-email_verification_enabled="{ value }">
      <NAlert
        v-if="value && mailEnabled === false"
        type="warning"
        class="mail-warning"
        data-testid="email-verification-needs-mail"
      >
        {{ t("admin.settings.emailVerificationNeedsMail") }}
        <RouterLink :to="{ name: 'admin-settings-mail' }">{{
          t("admin.settings.sections.mail.title")
        }}</RouterLink>
      </NAlert>
    </template>
  </SettingsFieldsForm>
  <CaptchaSettingsCard />
</template>

<style scoped>
.mail-warning {
  margin-top: 8px;
}
</style>
