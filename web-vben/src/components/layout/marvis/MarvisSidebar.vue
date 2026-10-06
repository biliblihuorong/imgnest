<script setup lang="ts">
import { LogOut, Search, Settings } from "@vben/icons";
import { useI18n } from "@vben/locales";
import { computed } from "vue";
import { useRouter } from "vue-router";
import logo from "@/assets/imgnest-mark.svg";
import { useUserAvatar } from "@/components/account/useUserAvatar";
import { menuIcons } from "@/integrations/vben/menuIcons";
import { workspaceMenus, type WorkspaceMenu } from "@/integrations/vben/navigation";
import { landingPath } from "@/router/landing";
import { useAuthStore } from "@/stores/auth";
import { useSiteStore } from "@/stores/site";

/** Marvis 外壳的漂浮侧栏：只负责呈现与发出意图，搜索/设置/退出由布局处理。 */
defineProps<{ collapsed: boolean }>();
const emit = defineEmits<{
  navigate: [];
  "open-search": [];
  "open-settings": [];
  logout: [];
}>();

const { t } = useI18n();
const router = useRouter();
const auth = useAuthStore();
const site = useSiteStore();
const avatarUrl = useUserAvatar(computed(() => auth.user));
const user = computed(() => (auth.token ? auth.user : null));

interface Section {
  key: string;
  label?: string;
  items: WorkspaceMenu[];
}

/** 顶层菜单为一组；带子项的菜单（站点管理）展开成带标题的分组，自身不可点。 */
const sections = computed<Section[]>(() => {
  const menus = workspaceMenus(user.value?.role, site.galleryEnabled);
  const result: Section[] = [{ key: "main", items: menus.filter((menu) => !menu.children) }];
  for (const menu of menus)
    if (menu.children) result.push({ key: menu.path, label: menu.name, items: menu.children });
  return result.filter((section) => section.items.length > 0);
});

const displayName = computed(
  () => user.value?.display_name || user.value?.username || t("common.account"),
);
const shortcut = /mac/i.test(navigator.userAgent) ? "⌘K" : "Ctrl K";

function goHome(): void {
  emit("navigate");
  void router.push(user.value ? landingPath(user.value.role) : "/gallery");
}
</script>

<template>
  <aside class="mv-sidebar" :class="{ 'is-collapsed': collapsed }">
    <button type="button" class="mv-brand" :aria-label="site.siteName" @click="goHome">
      <img class="mv-brand__mark" :src="logo" alt="" />
      <template v-if="!collapsed">
        <b class="mv-brand__name">{{ site.siteName }}</b>
        <span class="mv-brand__badge">{{ t("shell.brandBadge") }}</span>
      </template>
    </button>

    <button
      v-if="user"
      type="button"
      class="mv-search"
      data-testid="sidebar-search"
      :aria-label="t('shell.search')"
      @click="emit('open-search')"
    >
      <Search class="mv-icon" />
      <template v-if="!collapsed">
        <span class="mv-search__text">{{ t("shell.search") }}</span>
        <kbd>{{ shortcut }}</kbd>
      </template>
    </button>

    <nav class="mv-nav" :aria-label="site.siteName">
      <template v-for="section in sections" :key="section.key">
        <div
          v-if="section.label && !collapsed"
          class="mv-nav__label"
          :data-testid="`sidebar-group-${section.key}`"
        >
          {{ t(section.label) }}
        </div>
        <template v-for="item in section.items" :key="item.path">
          <RouterLink
            :to="item.path"
            class="mv-nav__item"
            :aria-label="collapsed ? t(item.name) : undefined"
            :title="collapsed ? t(item.name) : undefined"
            @click="emit('navigate')"
          >
            <component :is="menuIcons[item.icon]" class="mv-icon" />
            <span v-if="!collapsed" class="mv-nav__text">{{ t(item.name) }}</span>
          </RouterLink>
          <slot v-if="item.path === '/albums' && !collapsed" name="albums" />
        </template>
      </template>
    </nav>

    <div class="mv-footer">
      <template v-if="user">
        <img class="mv-footer__avatar" :src="avatarUrl" alt="" />
        <span v-if="!collapsed" class="mv-footer__name" :title="user.email ?? ''">{{
          displayName
        }}</span>
      </template>
      <button
        v-else
        type="button"
        class="mv-footer__login"
        data-testid="sidebar-login"
        @click="router.push('/login')"
      >
        {{ t("shell.login") }}
      </button>
      <!-- 匿名访客也能切换深浅色、语言和布局。 -->
      <button
        type="button"
        class="mv-icon-btn"
        data-testid="sidebar-settings"
        :aria-label="t('shell.openSettings')"
        :title="t('shell.openSettings')"
        @click="emit('open-settings')"
      >
        <Settings class="mv-icon" />
      </button>
      <template v-if="user">
        <button
          type="button"
          class="mv-icon-btn"
          data-testid="sidebar-logout"
          :aria-label="t('shell.logout')"
          :title="t('shell.logout')"
          @click="emit('logout')"
        >
          <LogOut class="mv-icon" />
        </button>
      </template>
    </div>
  </aside>
</template>

<style scoped>
.mv-sidebar {
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  width: 244px;
  margin: 14px 0 14px 14px;
  padding: 20px 14px 14px;
  overflow-y: auto;
  user-select: none;
  background: hsl(var(--sidebar));
  border: 1px solid hsl(var(--border) / 0.6);
  border-radius: 20px;
  box-shadow: var(--mv-shadow-side);
}
.mv-sidebar.is-collapsed {
  width: 68px;
  padding-inline: 10px;
}
.mv-icon {
  flex-shrink: 0;
  width: 16px;
  height: 16px;
}
.mv-brand {
  display: flex;
  gap: 8px;
  align-items: center;
  padding: 2px 10px 16px;
  text-align: left;
}
.mv-brand__mark {
  width: 24px;
  height: 24px;
  border-radius: 8px;
}
.mv-brand__name {
  overflow: hidden;
  font-size: 21px;
  font-weight: 800;
  text-overflow: ellipsis;
  letter-spacing: -0.02em;
  white-space: nowrap;
}
.mv-brand__badge {
  flex-shrink: 0;
  padding: 2px 8px;
  font-size: 10px;
  font-weight: 600;
  color: hsl(var(--muted-foreground));
  letter-spacing: 0.04em;
  white-space: nowrap;
  background: hsl(var(--muted));
  border-radius: 99px;
}
.mv-search {
  display: flex;
  gap: 8px;
  align-items: center;
  padding: 8px 11px;
  margin-bottom: 8px;
  font-size: 13px;
  color: hsl(var(--muted-foreground));
  text-align: left;
  background: hsl(var(--muted));
  border-radius: 10px;
  transition: background 0.2s;
}
.mv-search:hover {
  background: hsl(var(--accent-dark, var(--muted)));
}
.mv-search__text {
  flex: 1;
}
.mv-search kbd {
  padding: 1px 5px;
  font-family: inherit;
  font-size: 10px;
  background: hsl(var(--card));
  border: 1px solid hsl(var(--border));
  border-radius: 5px;
}
.mv-nav {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 2px;
  min-height: 0;
}
.mv-nav__label {
  padding: 20px 10px 6px;
  font-size: 11px;
  font-weight: 600;
  color: hsl(var(--muted-foreground));
  letter-spacing: 0.06em;
}
.mv-nav__item {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 8px 10px;
  font-size: 13.5px;
  font-weight: 500;
  color: hsl(var(--foreground) / 0.82);
  border-radius: 10px;
  transition: background 0.18s var(--mv-ease);
}
.mv-nav__item:hover {
  background: hsl(var(--muted) / 0.7);
}
.mv-nav__item.router-link-active {
  font-weight: 600;
  color: hsl(var(--foreground));
  background: hsl(var(--muted));
}
.mv-nav__text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mv-footer {
  display: flex;
  flex-shrink: 0;
  gap: 6px;
  align-items: center;
  padding: 8px 10px;
  margin-top: 12px;
  background: hsl(var(--background-deep));
  border-radius: 14px;
}
.mv-footer__avatar {
  flex-shrink: 0;
  width: 28px;
  height: 28px;
  object-fit: cover;
  border-radius: 50%;
}
.mv-footer__name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  font-size: 13px;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mv-footer__login {
  flex: 1;
  padding: 6px 0;
  font-size: 13px;
  font-weight: 600;
}
.mv-icon-btn {
  display: grid;
  flex-shrink: 0;
  place-items: center;
  width: 30px;
  height: 30px;
  color: hsl(var(--muted-foreground));
  border-radius: 9px;
  transition: background 0.15s;
}
.mv-icon-btn:hover {
  color: hsl(var(--foreground));
  background: hsl(var(--muted));
}
.is-collapsed .mv-brand,
.is-collapsed .mv-search,
.is-collapsed .mv-nav__item {
  justify-content: center;
  padding-inline: 0;
}
.is-collapsed .mv-footer {
  flex-direction: column;
  padding: 8px 4px;
}
</style>
