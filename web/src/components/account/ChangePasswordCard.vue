<script setup lang="ts">
import {
  NAlert,
  NButton,
  NForm,
  NFormItem,
  NInput,
  useMessage,
  type FormInst,
  type FormItemRule,
  type FormRules,
} from "naive-ui";
import { reactive, ref } from "vue";
import { useRouter } from "vue-router";
import { changePassword } from "@/api/auth";
import { useAuthStore } from "@/stores/auth";

/** 后端规则：新密码 12–72 UTF-8 字节；客户端先做等价的长度前置校验。 */
const MIN_PASSWORD_CHARS = 12;
const MAX_PASSWORD_BYTES = 72;

const message = useMessage();
const router = useRouter();
const auth = useAuthStore();

const formRef = ref<FormInst | null>(null);
const model = reactive({ current: "", next: "", confirm: "" });
const submitting = ref(false);
const serverError = ref<string | null>(null);

function byteLength(value: string): number {
  return new TextEncoder().encode(value).length;
}

function validateNewPassword(_rule: FormItemRule, value: string): boolean | Error {
  if (value.length < MIN_PASSWORD_CHARS) {
    return new Error(`新密码至少 ${MIN_PASSWORD_CHARS} 个字符`);
  }
  if (byteLength(value) > MAX_PASSWORD_BYTES) {
    return new Error(`新密码最长 ${MAX_PASSWORD_BYTES} 字节`);
  }
  return true;
}

const rules: FormRules = {
  current: [{ required: true, message: "请输入当前密码", trigger: ["blur", "input"] }],
  next: [
    { required: true, message: "请输入新密码", trigger: ["blur", "input"] },
    { validator: validateNewPassword, trigger: ["blur", "input"] },
  ],
  confirm: [
    { required: true, message: "请再次输入新密码", trigger: ["blur", "input"] },
    {
      validator: (_rule: FormItemRule, value: string): boolean | Error =>
        value === model.next ? true : new Error("两次输入的密码不一致"),
      trigger: ["blur", "input"],
    },
  ],
};

async function onSubmit(): Promise<void> {
  if (submitting.value) {
    return;
  }
  serverError.value = null;
  try {
    await formRef.value?.validate();
  } catch {
    // 客户端校验未通过，错误已就地展示，不发请求
    return;
  }
  submitting.value = true;
  try {
    await changePassword(model.current, model.next);
    message.success("密码已修改，请重新登录");
    // 服务端已吊销该用户全部 Token；只清本地登录态，不调 logout 重复吊销
    auth.clear();
    await router.push("/login");
  } catch (error) {
    // 失败（含 20002 旧密码错误）就地展示，不清登录态：此时会话仍然有效
    serverError.value =
      error instanceof Error && error.message ? error.message : "修改密码失败，请稍后重试";
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <NForm
    ref="formRef"
    class="change-password"
    :model="model"
    :rules="rules"
    label-placement="top"
    @submit.prevent="onSubmit"
  >
    <NFormItem label="当前密码" path="current">
      <NInput
        v-model:value="model.current"
        type="password"
        show-password-on="click"
        placeholder="当前密码"
        :input-props="{ autocomplete: 'current-password' }"
        :disabled="submitting"
      />
    </NFormItem>
    <NFormItem label="新密码（12–72 字节）" path="next">
      <NInput
        v-model:value="model.next"
        type="password"
        show-password-on="click"
        placeholder="新密码（至少 12 位）"
        :input-props="{ autocomplete: 'new-password' }"
        :disabled="submitting"
      />
    </NFormItem>
    <NFormItem label="确认新密码" path="confirm">
      <NInput
        v-model:value="model.confirm"
        type="password"
        show-password-on="click"
        placeholder="再次输入新密码"
        :input-props="{ autocomplete: 'new-password' }"
        :disabled="submitting"
      />
    </NFormItem>
    <NAlert v-if="serverError" type="error" class="change-password__error">
      {{ serverError }}
    </NAlert>
    <NButton attr-type="submit" type="primary" :loading="submitting">修改密码</NButton>
  </NForm>
</template>
