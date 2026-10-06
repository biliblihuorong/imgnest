<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NButton } from "naive-ui";
import { landingPath } from "@/router/landing";
import { BasicLayout, UserDropdown } from "@vben/layouts";
import { useRouter } from "vue-router";
import { computed } from "vue";
import { useAuthStore } from "@/stores/auth";
import { useSiteStore } from "@/stores/site";
import { useUserAvatar } from "@/components/account/useUserAvatar";
import logo from "@/assets/imgnest-mark.svg";
import { setShell } from "@/integrations/shell/useShell";
import { useLogout } from "./useLogout";

const { t } = useI18n();
const router = useRouter();
const auth = useAuthStore();
const site = useSiteStore();
const { logout } = useLogout();
void site.ensureLoaded();
const avatarUrl = useUserAvatar(computed(() => auth.user));

const accountMenus = computed(() => [
  { text: t("shell.switchToMarvis"), handler: () => setShell("marvis") },
  { text: t("account.profileNav"), handler: () => router.push("/account/settings") },
  { text: t("common.nav.tokens"), handler: () => router.push("/tokens") },
  ...(auth.user?.role === "admin"
    ? [{ text: t("common.nav.admin"), handler: () => router.push("/admin/users") }]
    : []),
]);
</script>

<template>
  <BasicLayout
    :refresh-on-locale-change="false"
    :logo-src="logo"
    :logo-src-dark="logo"
    :logo-text="site.siteName"
    :avatar="avatarUrl"
    :text="auth.user?.display_name || auth.user?.username || t('common.account')"
    @logout="logout"
    @clear-preferences-and-logout="logout"
    @click-logo="router.push(auth.user ? landingPath(auth.user.role) : '/gallery')"
  >
    <template #user-dropdown>
      <UserDropdown
        v-if="auth.user"
        :avatar="avatarUrl"
        :avatar-dot="false"
        :text="auth.user?.display_name || auth.user?.username || t('common.account')"
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
