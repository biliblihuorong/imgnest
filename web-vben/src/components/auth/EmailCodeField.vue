<script setup lang="ts">
// 邮件验证码输入：先过人机验证（若开启）再发送，发送后 60 秒内不能重发。
import { useI18n } from "@vben/locales";
import { NButton, NFormItem, NInput, NText } from "naive-ui";
import { computed, onBeforeUnmount, ref, shallowRef, useTemplateRef } from "vue";
import { sendEmailCode, type EmailCodePurpose } from "@/api/auth";
import AuthCaptcha from "@/components/captcha/AuthCaptcha.vue";
import { formatApiError } from "@/locales/errors";

const props = defineProps<{ email: string; purpose: EmailCodePurpose; disabled?: boolean }>();
const code = defineModel<string>("code", { required: true });
const emit = defineEmits<{ sent: [] }>();

const COOLDOWN_SECONDS = 60;
const { t } = useI18n();
const captcha = useTemplateRef<InstanceType<typeof AuthCaptcha>>("captcha");
const captchaReady = ref(false);
const sending = shallowRef(false);
const cooldown = shallowRef(0);
const sent = shallowRef(false);
const error = shallowRef<unknown>(null);
let timer: ReturnType<typeof setInterval> | undefined;

const emailLooksValid = computed(() => /^[^\s@]+@[^\s@]+$/.test(props.email.trim()));
const canSend = computed(
  () =>
    !props.disabled &&
    !sending.value &&
    cooldown.value === 0 &&
    captchaReady.value &&
    emailLooksValid.value,
);
const errorMessage = computed(() =>
  error.value ? formatApiError(error.value, "common.errors.unknown") : "",
);

function startCooldown(): void {
  cooldown.value = COOLDOWN_SECONDS;
  clearInterval(timer);
  timer = setInterval(() => {
    cooldown.value = Math.max(0, cooldown.value - 1);
    if (cooldown.value === 0) clearInterval(timer);
  }, 1000);
}

async function send(): Promise<void> {
  if (!canSend.value) return;
  sending.value = true;
  error.value = null;
  try {
    const token = captcha.value!.consume();
    await sendEmailCode(props.email.trim(), props.purpose, token);
    sent.value = true;
    startCooldown();
    emit("sent");
  } catch (err) {
    error.value = err;
    await captcha.value?.reload();
  } finally {
    sending.value = false;
  }
}

onBeforeUnmount(() => clearInterval(timer));
</script>

<template>
  <AuthCaptcha
    ref="captcha"
    :action="purpose === 'register' ? 'register' : 'reset'"
    @ready="captchaReady = $event"
  />
  <NFormItem :label="t('common.auth.emailCode')">
    <div class="code-row">
      <NInput
        v-model:value="code"
        :maxlength="6"
        :placeholder="t('common.auth.emailCodeHint')"
        :disabled="disabled"
        :input-props="{
          autocomplete: 'one-time-code',
          inputmode: 'numeric',
          'aria-label': t('common.auth.emailCode'),
        }"
      />
      <NButton
        attr-type="button"
        data-action="send-code"
        :loading="sending"
        :disabled="!canSend"
        @click="send"
      >
        {{
          cooldown > 0
            ? t("common.auth.resendIn", { seconds: cooldown })
            : t("common.auth.sendCode")
        }}
      </NButton>
    </div>
  </NFormItem>
  <p v-if="errorMessage" role="alert" class="code-message">
    <NText type="error">{{ errorMessage }}</NText>
  </p>
  <p v-else-if="sent" role="status" class="code-message">
    <NText type="success">{{
      purpose === "register" ? t("common.auth.codeSent") : t("common.auth.resetCodeSent")
    }}</NText>
  </p>
</template>

<style scoped>
.code-row {
  display: flex;
  gap: 10px;
  width: 100%;
}
.code-row :deep(.n-button) {
  flex: 0 0 auto;
}
.code-message {
  margin: -12px 0 16px;
  font-size: 13px;
}
</style>
