<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NButton, NResult } from "naive-ui";
import { useRouter } from "vue-router";
import { homePath } from "@/app.hooks";
import { useAuthStore } from "@/stores/auth";

const { t } = useI18n();
const router = useRouter();
const auth = useAuthStore();
</script>

<template>
  <NResult
    status="403"
    :title="t('common.forbidden')"
    :description="t('common.forbiddenDescription')"
    style="margin: 48px auto"
  >
    <template #footer>
      <NButton type="primary" @click="router.push(homePath(auth.user))">{{
        t("common.home")
      }}</NButton>
    </template>
  </NResult>
</template>
