<script setup lang="ts">
import { Page } from "@vben/common-ui";
import { useI18n } from "@vben/locales";
import {
  NAlert,
  NAvatar,
  NButton,
  NCard,
  NForm,
  NFormItem,
  NInput,
  useMessage,
  type FormInst,
  type FormItemRule,
  type FormRules,
} from "naive-ui";
import { computed, reactive, ref, shallowRef, watch } from "vue";
import { updateDisplayName } from "@/api/auth";
import ChangePasswordCard from "@/components/account/ChangePasswordCard.vue";
import { useAccountScope } from "@/components/account/useAccountScope";
import { useUserAvatar } from "@/components/account/useUserAvatar";
import { formatApiError } from "@/locales/errors";
import { useAuthStore } from "@/stores/auth";

const { t, locale } = useI18n();
const message = useMessage();
const auth = useAuthStore();
const avatarUrl = useUserAvatar(computed(() => auth.user), 320);

const formRef = ref<FormInst | null>(null);
const model = reactive({ display_name: "" });
const submitting = shallowRef(false);
const serverError = shallowRef<unknown>(null);
const errorText = computed(() => {
  void locale.value;
  return serverError.value ? formatApiError(serverError.value, "account.profile.error") : null;
});

// 表单跟随当前会话用户；账户切换或保存成功后由 store 驱动回填。
watch(
  () => auth.user?.display_name,
  (name) => {
    model.display_name = name ?? "";
  },
  { immediate: true },
);

const scope = useAccountScope(() => {
  serverError.value = null;
});

const dirty = computed(() => model.display_name !== (auth.user?.display_name ?? ""));

const rules: FormRules = {
  display_name: [
    {
      trigger: ["blur", "input"],
      validator: (_rule: FormItemRule, value: string): boolean | Error =>
        [...(value ?? "").trim()].length > 64 ? new Error(t("account.profile.tooLong")) : true,
    },
  ],
};

async function onSave(): Promise<void> {
  if (submitting.value || !scope.available.value) return;
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
    const updated = await updateDisplayName(model.display_name);
    // 迟到的成功响应不得写回别的会话。
    if (!scope.isCurrent(version)) return;
    auth.setUser(updated);
    message.success(() => t("account.profile.saved"));
  } catch (error) {
    if (scope.isCurrent(version)) serverError.value = error;
  } finally {
    submitting.value = false;
  }
}

const shownName = computed(() => auth.user?.display_name || auth.user?.username || "");
</script>

<template>
  <Page
    :title="t('account.settingsTitle')"
    :description="t('account.profile.description')"
    class="account-settings min-w-0"
    header-class="flex-wrap items-center gap-4"
    content-class="min-w-0"
  >
    <NCard :bordered="false">
      <div class="account-settings__identity">
        <NAvatar round :size="72" :src="avatarUrl" :alt="t('account.profile.avatarAlt')" />
        <div class="account-settings__who">
          <p class="account-settings__name">{{ shownName }}</p>
          <p class="account-settings__meta">
            {{ t("account.profile.username") }}：{{ auth.user?.username }}
          </p>
          <p class="account-settings__meta">
            {{ t("account.profile.email") }}：{{ auth.user?.email }}
          </p>
        </div>
      </div>
      <NForm
        ref="formRef"
        class="account-settings__form"
        :model="model"
        :rules="rules"
        label-placement="top"
        @submit.prevent="onSave"
      >
        <NFormItem :label="t('account.profile.displayName')" path="display_name">
          <NInput
            v-model:value="model.display_name"
            maxlength="64"
            show-count
            :placeholder="t('account.profile.displayNamePlaceholder')"
            :disabled="submitting || !scope.available.value"
          />
          <p class="account-settings__hint">{{ t("account.profile.displayNameHint") }}</p>
        </NFormItem>
        <NAlert v-if="errorText" type="error" class="account-settings__error">{{ errorText }}</NAlert>
        <NButton
          type="primary"
          :loading="submitting"
          :disabled="!dirty || !scope.available.value"
          @click="onSave"
          >{{ t("account.profile.save") }}</NButton
        >
        <p class="account-settings__avatar-note">{{ t("account.profile.avatarNote") }}</p>
      </NForm>
    </NCard>
    <NCard :title="t('account.securityTitle')" :bordered="false" class="mt-5">
      <p class="account-settings__security-hint">{{ t("account.securityHint") }}</p>
      <ChangePasswordCard />
    </NCard>
  </Page>
</template>

<style scoped>
.account-settings__identity {
  display: flex;
  align-items: center;
  gap: 20px;
  margin-bottom: 24px;
}
.account-settings__who {
  min-width: 0;
}
.account-settings__name {
  margin: 0 0 4px;
  font-size: 18px;
  font-weight: 650;
}
.account-settings__meta {
  margin: 0;
  color: hsl(var(--muted-foreground));
  font-size: 13px;
}
.account-settings__form {
  max-width: 520px;
}
.account-settings__hint {
  margin: 4px 0 0;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}
.account-settings__error {
  margin-bottom: 16px;
}
.account-settings__avatar-note {
  margin: 16px 0 0;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}
.account-settings__security-hint {
  margin: 0 0 20px;
  color: hsl(var(--muted-foreground));
  font-size: 13px;
}
</style>
