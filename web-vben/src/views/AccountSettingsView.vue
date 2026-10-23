<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NCard } from "naive-ui";
import AccountPage from "@/components/account/AccountPage.vue";
import ChangePasswordCard from "@/components/account/ChangePasswordCard.vue";
import ProfileCard from "@/components/account/ProfileCard.vue";
import UserPluginSettings from "@/components/account/UserPluginSettings.vue";

const { t } = useI18n();
</script>

<template>
  <AccountPage class="account-settings">
    <NCard :bordered="false">
      <ProfileCard />
    </NCard>
    <NCard :title="t('account.securityTitle')" :bordered="false" class="mt-5">
      <p class="account-settings__security-hint">{{ t("account.securityHint") }}</p>
      <ChangePasswordCard />
    </NCard>
    <UserPluginSettings />
  </AccountPage>
</template>

<style scoped>
.account-settings__security-hint {
  margin: 0 0 20px;
  color: hsl(var(--muted-foreground));
  font-size: 13px;
}
</style>
