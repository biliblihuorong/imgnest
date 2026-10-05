<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NButton } from "naive-ui";
import { landingPath } from "@/router/landing";
import { BasicLayout, UserDropdown } from "@vben/layouts";
import { useRouter } from "vue-router";
import { computed, shallowRef } from "vue";
import { useAuthStore } from "@/stores/auth";
import { useSiteStore } from "@/stores/site";
import logo from "@/assets/imgnest-mark.svg";
import avatar from "@/assets/account-avatar.svg";

const { t } = useI18n();
const router = useRouter();
const auth = useAuthStore();
const site = useSiteStore();
const loggingOut = shallowRef(false);
void site.ensureLoaded();

const accountMenus = computed(() => [
  { text: t("common.nav.tokens"), handler: () => router.push("/tokens") },
  ...(auth.user?.role === "admin"
    ? [{ text: t("common.nav.admin"), handler: () => router.push("/admin/users") }]
    : []),
]);

async function logout(): Promise<void> {
  if (loggingOut.value) return;
  loggingOut.value = true;
  try {
    const request = auth.logout();
    // Leave the sensitive view immediately. A delayed logout response must never
    // navigate a newer session away from its current workspace.
    await router.replace("/login");
    await request;
  } finally {
    loggingOut.value = false;
  }
}
</script>

<template>
  <BasicLayout
    :refresh-on-locale-change="false"
    :logo-src="logo"
    :logo-src-dark="logo"
    :logo-text="site.siteName"
    :avatar="avatar"
    :text="auth.user?.username ?? t('common.account')"
    @logout="logout"
    @clear-preferences-and-logout="logout"
    @click-logo="router.push(auth.user ? landingPath(auth.user.role) : '/gallery')"
  >
    <template #user-dropdown>
      <UserDropdown
        v-if="auth.user"
        :avatar="avatar"
        :avatar-dot="false"
        :text="auth.user?.username ?? t('common.account')"
        :description="auth.user?.email ?? ''"
        :tag-text="auth.user?.role === 'admin' ? t('common.administrator') : t('common.user')"
        :menus="accountMenus"
        @logout="logout"
        @clear-preferences-and-logout="logout"
      />
      <NButton v-else secondary @click="router.push('/login')">{{ t("common.login") }}</NButton>
    </template>
  </BasicLayout>
</template>
