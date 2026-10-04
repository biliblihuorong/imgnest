<script setup lang="ts">
import { NButton, NCard, NForm, NFormItem, NInput, NResult, NText, useMessage } from "naive-ui";
import { computed, ref } from "vue";
import { useRouter } from "vue-router";
import { register } from "@/api/auth";
import { ApiError } from "@/api/client";
import { useSiteStore } from "@/stores/site";

const router = useRouter();
const site = useSiteStore();
const message = useMessage();

const username = ref("");
const email = ref("");
const password = ref("");
const confirmPassword = ref("");
const submitting = ref(false);
const errorMessage = ref("");

void site.ensureLoaded();

// 后端密码规则：12-72 字节；字节上限客户端预检，超限不发请求
const passwordBytes = computed(() => new TextEncoder().encode(password.value).length);

const canSubmit = computed(
  () =>
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
    errorMessage.value = "两次输入的密码不一致";
    return;
  }
  submitting.value = true;
  errorMessage.value = "";
  try {
    await register({
      username: username.value.trim(),
      email: email.value.trim(),
      password: password.value,
    });
    message.success("注册成功，请登录");
    await router.push("/login");
  } catch (error) {
    errorMessage.value = error instanceof ApiError ? error.message : "网络错误";
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <main class="register-page">
    <NCard
      v-if="site.registerEnabled"
      class="register-card"
      :title="`注册 ${site.siteName} 账号`"
      bordered
    >
      <NForm label-placement="top" @submit.prevent="onSubmit">
        <NFormItem label="用户名">
          <NInput v-model:value="username" placeholder="用户名（3-64 个字符）" />
        </NFormItem>
        <NFormItem label="邮箱">
          <NInput
            v-model:value="email"
            placeholder="邮箱"
            :input-props="{ autocomplete: 'email' }"
          />
        </NFormItem>
        <NFormItem label="密码">
          <NInput
            v-model:value="password"
            type="password"
            show-password-on="click"
            placeholder="密码（至少 12 个字符）"
            :input-props="{ autocomplete: 'new-password' }"
          />
        </NFormItem>
        <NFormItem label="确认密码">
          <NInput
            v-model:value="confirmPassword"
            type="password"
            show-password-on="click"
            placeholder="再次输入密码"
            :input-props="{ autocomplete: 'new-password' }"
          />
        </NFormItem>
        <p v-if="errorMessage" role="alert">
          <NText type="error">{{ errorMessage }}</NText>
        </p>
        <NButton
          attr-type="submit"
          type="primary"
          block
          :loading="submitting"
          :disabled="!canSubmit"
        >
          注册
        </NButton>
      </NForm>
      <p class="register-footer">已有账号？<RouterLink to="/login">返回登录</RouterLink></p>
    </NCard>
    <NCard v-else class="register-card" :title="site.siteName" bordered>
      <NResult status="403" title="未开放注册" description="当前站点未开放注册，请联系管理员。" />
      <p class="register-footer"><RouterLink to="/login">返回登录</RouterLink></p>
    </NCard>
  </main>
</template>

<style scoped>
.register-page {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  padding: 16px;
}

.register-card {
  width: 100%;
  max-width: 380px;
}

.register-footer {
  margin: 12px 0 0;
  text-align: center;
}
</style>
