<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NButton, NTag } from "naive-ui";
import { shallowRef, watch } from "vue";
import type { CaptchaAction, PublicCaptchaConfig } from "@/api/captcha";
import CaptchaChallenge from "./CaptchaChallenge.vue";

const props = defineProps<{
  action: CaptchaAction;
  config: PublicCaptchaConfig | null;
  disabled: boolean;
  tested: boolean;
}>();
const emit = defineEmits<{ verify: [action: CaptchaAction, token: string] }>();
const { t } = useI18n();
const started = shallowRef(false);
const token = shallowRef<string | null>(null);
const resetKey = shallowRef(0);
function verify(): void {
  if (props.disabled || !token.value) return;
  const response = token.value;
  token.value = null;
  resetKey.value++;
  emit("verify", props.action, response);
}
watch(
  () => [props.disabled, props.config?.version],
  () => {
    token.value = null;
  },
);
</script>

<template>
  <section class="draft-test" :aria-label="t(`captcha.admin.actions.${action}`)">
    <div class="test-heading">
      <h4>{{ t(`captcha.admin.actions.${action}`) }}</h4>
      <NTag :type="tested ? 'success' : 'default'" :data-action-state="action" size="small">{{
        t(tested ? "captcha.admin.passed" : "captcha.admin.notTested")
      }}</NTag>
    </div>
    <NButton
      v-if="!started"
      attr-type="button"
      :data-action="`start-${action}`"
      :disabled="disabled"
      @click="started = true"
      >{{ t("captcha.admin.startTest") }}</NButton
    >
    <template v-else>
      <CaptchaChallenge
        v-if="config && !disabled"
        :config="config"
        :action="action"
        :reset-key="resetKey"
        size="compact"
        @token="token = $event"
      />
      <NButton
        attr-type="button"
        :data-action="`verify-${action}`"
        :disabled="disabled || !token"
        @click="verify"
        >{{ t("captcha.admin.verifyTest") }}</NButton
      >
    </template>
  </section>
</template>

<style scoped>
.draft-test {
  padding: 16px;
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
  min-width: 0;
}
.test-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.test-heading h4 {
  margin: 0;
}
</style>
