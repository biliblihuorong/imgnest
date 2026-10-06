<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { landingPath } from "@/router/landing";
import { useAuthStore } from "@/stores/auth";
import { NButton, NResult } from "naive-ui";
import { useRouter } from "vue-router";

const { t } = useI18n();
const auth = useAuthStore();
const router = useRouter();

async function backToUpload(): Promise<void> {
  await router.push(landingPath(auth.user?.role));
}
</script>

<template>
  <NResult
    status="403"
    :title="t('common.forbidden')"
    :description="t('common.forbiddenDescription')"
    style="margin: 48px auto"
  >
    <template #footer>
      <NButton type="primary" @click="backToUpload">{{ t("common.home") }}</NButton>
    </template>
  </NResult>
</template>
