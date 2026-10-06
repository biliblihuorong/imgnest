<script setup lang="ts">
import { Page } from "@vben/common-ui";
import { useI18n } from "@vben/locales";
import { NCard } from "naive-ui";
import ChangePasswordCard from "@/components/account/ChangePasswordCard.vue";
import ProfileCard from "@/components/account/ProfileCard.vue";

const { t } = useI18n();
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
      <ProfileCard />
    </NCard>
    <NCard :title="t('account.securityTitle')" :bordered="false" class="mt-5">
      <p class="account-settings__security-hint">{{ t("account.securityHint") }}</p>
      <ChangePasswordCard />
    </NCard>
  </Page>
</template>

<style scoped>
.account-settings__security-hint {
  margin: 0 0 20px;
  color: hsl(var(--muted-foreground));
  font-size: 13px;
}
</style>
