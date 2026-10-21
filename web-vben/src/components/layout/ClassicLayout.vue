<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { Settings } from "@vben/icons";
import { NButton } from "naive-ui";
import { landingPath } from "@/router/landing";
import { BasicLayout, UserDropdown } from "@vben/layouts";
import { useRouter } from "vue-router";
import { computed, shallowRef } from "vue";
import { useAuthStore } from "@/stores/auth";
import { useSiteStore } from "@/stores/site";
import { useUserAvatar } from "@/components/account/useUserAvatar";
import logo from "@/assets/imgnest-mark.svg";
import { setShell } from "@/integrations/shell/useShell";
import MarvisSettingsModal from "./marvis/MarvisSettingsModal.vue";
import { useLogout } from "./useLogout";

const { t } = useI18n();
const router = useRouter();
const auth = useAuthStore();
const site = useSiteStore();
const { logout } = useLogout();
void site.ensureLoaded();
const avatarUrl = useUserAvatar(computed(() => auth.user));
const settingsOpen = shallowRef(false);

const accountMenus = computed(() => [
  { text: t("shell.switchToMarvis"), handler: () => setShell("marvis") },
  { text: t("common.nav.tokens"), handler: () => router.push("/account") },
  { text: t("shell.settings.title"), handler: () => (settingsOpen.value = true) },
  ...(auth.user?.role === "admin"
    ? [{ text: t("common.nav.admin"), handler: () => router.push("/admin/overview") }]
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
    <!-- 原偏好按钮的位置（全局搜索之后）；打开与新布局相同的设置弹窗。 -->
    <template #header-right-100>
      <button
        type="button"
        class="classic-settings-btn"
        data-testid="classic-settings-button"
        :title="t('shell.settings.title')"
        :aria-label="t('shell.settings.title')"
        @click="settingsOpen = true"
      >
        <Settings class="classic-settings-btn__icon" />
      </button>
    </template>
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
  <MarvisSettingsModal v-model:show="settingsOpen" />
</template>

<style scoped>
.classic-settings-btn {
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  margin-right: 4px;
  color: hsl(var(--foreground));
  border-radius: calc(var(--radius) + 2px);
}
.classic-settings-btn:hover {
  background: hsl(var(--accent));
}
.classic-settings-btn__icon {
  width: 16px;
  height: 16px;
}
</style>
