<script setup lang="ts">
import { NButton, NCard, NForm, NFormItem, NInput, NText } from "naive-ui";
import { computed, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { ApiError } from "@/api/client";
import { useAuthStore } from "@/stores/auth";
import { useSiteStore } from "@/stores/site";

const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const site = useSiteStore();

const email = ref("");
const password = ref("");
const submitting = ref(false);
const errorMessage = ref("");

const canSubmit = computed(() => email.value.trim().length > 0 && password.value.length > 0);

void site.ensureLoaded();

/** 只允许站内路径，避免 redirect 参数把用户带出应用。 */
function sanitizeRedirect(target: unknown): string {
  if (typeof target === "string" && target.startsWith("/") && !target.startsWith("//")) {
    return target;
  }
  return "/upload";
}

async function onSubmit(): Promise<void> {
  if (submitting.value || !canSubmit.value) {
    return;
  }
  submitting.value = true;
  errorMessage.value = "";
  try {
    await auth.login(email.value.trim(), password.value);
    await router.push(sanitizeRedirect(route.query.redirect));
  } catch (error) {
    errorMessage.value = error instanceof ApiError ? error.message : "网络错误";
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <main class="login-page">
    <NCard class="login-card" :title="site.siteName" bordered>
      <NForm label-placement="top" @submit.prevent="onSubmit">
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
            placeholder="密码"
            :input-props="{ autocomplete: 'current-password' }"
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
          登录
        </NButton>
      </NForm>
      <p v-if="site.registerEnabled" class="login-footer">
        还没有账号？<RouterLink to="/register">注册账号</RouterLink>
      </p>
    </NCard>
  </main>
</template>

<style scoped>
.login-page {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  padding: 16px;
}

.login-card {
  width: 100%;
  max-width: 380px;
}

.login-footer {
  margin: 12px 0 0;
  text-align: center;
}
</style>
