<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { Settings } from "@vben/icons";
import { BasicLayout, UserDropdown } from "@vben/layouts";
import { NButton } from "naive-ui";
import { useRouter } from "vue-router";
import { computed, shallowRef } from "vue";
import { appConfig } from "@/app.config";
import { homePath, shellExtensions } from "@/app.hooks";
import MarvisSettingsModal from "./marvis/MarvisSettingsModal.vue";
import { site } from "./site";
import { setShell } from "./useShell";
import { useCurrentUser } from "./useCurrentUser";
import { useLogout } from "./useLogout";

/**
 * 经典外壳：Vben 原生的顶栏 + 侧栏菜单 + 标签页，菜单来自 session.ts 的投影。
 * 设置用和 Marvis 相同的设置弹窗（Vben 自带的偏好抽屉已关闭）。
 */
const { t } = useI18n();
const router = useRouter();
const { logout } = useLogout();
const { user, displayName, avatar } = useCurrentUser();
const settingsOpen = shallowRef(false);

const accountMenus = computed(() => [
  ...(appConfig.accountPath
    ? [{ text: t("common.account"), handler: () => router.push(appConfig.accountPath) }]
    : []),
  ...(user.value && shellExtensions.userMenu
    ? shellExtensions.userMenu(user.value).map((link) => ({
        text: t(link.label),
        handler: () => router.push(link.path),
      }))
    : []),
  { text: t("shell.settings.title"), handler: () => (settingsOpen.value = true) },
  { text: t("shell.switchToMarvis"), handler: () => setShell("marvis") },
]);
</script>

<template>
  <BasicLayout
    :refresh-on-locale-change="false"
    :logo-src="site.logo"
    :logo-src-dark="site.logo"
    :logo-text="site.name"
    :avatar="avatar"
    :text="displayName"
    @logout="logout"
    @clear-preferences-and-logout="logout"
    @click-logo="router.push(homePath(user))"
  >
    <!-- 原偏好按钮的位置（全局搜索之后）；打开与 Marvis 相同的设置弹窗。 -->
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
        v-if="user"
        :avatar="avatar"
        :avatar-dot="false"
        :text="displayName"
        :description="user.email ?? ''"
        :tag-text="t(`common.roles.${user.role}`)"
        :menus="accountMenus"
        @logout="logout"
        @clear-preferences-and-logout="logout"
      />
      <NButton v-else secondary @click="router.push(appConfig.loginPath)">{{
        t("common.login")
      }}</NButton>
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
