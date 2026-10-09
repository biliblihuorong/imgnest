<script setup lang="ts">
import { NButton, NSpin, NText } from "naive-ui";
import { onBeforeUnmount, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { useI18n } from "@vben/locales";
import { landingPath } from "@/router/landing";
import { useAuthStore } from "@/stores/auth";

const { t } = useI18n();
const router = useRouter();
const auth = useAuthStore();
const failed = ref(false);
const request = new AbortController();

/** 票据只放在 URL 片段里（不会发给服务器或写进 Referer），读到后立即从地址栏抹掉。 */
function takeTicket(): string {
  const params = new URLSearchParams(window.location.hash.slice(1));
  const ticket = params.get("ticket") ?? "";
  window.history.replaceState(window.history.state, "", window.location.pathname);
  return ticket;
}

onMounted(async () => {
  const ticket = takeTicket();
  if (!ticket) {
    failed.value = true;
    return;
  }
  try {
    const user = await auth.loginWithSsoTicket(ticket, request.signal);
    await router.replace(landingPath(user.role));
  } catch {
    if (!request.signal.aborted) failed.value = true;
  }
});
onBeforeUnmount(() => request.abort());
</script>

<template>
  <main class="sso-page">
    <template v-if="failed">
      <p role="alert">
        <NText type="error">{{ t("common.auth.ssoErrors.failed") }}</NText>
      </p>
      <NButton tag="a" href="/login" block>{{ t("common.auth.ssoBack") }}</NButton>
    </template>
    <NSpin v-else :description="t('common.auth.ssoSigningIn')" />
  </main>
</template>

<style scoped>
.sso-page {
  width: 100%;
  max-width: 420px;
  margin: auto;
  text-align: center;
}
</style>
