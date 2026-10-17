<script setup lang="ts">
import { Languages, LogOut, PanelLeft, Search, Settings } from "@vben/icons";
import { useI18n } from "@vben/locales";
import { NDropdown, type DropdownOption } from "naive-ui";
import { computed } from "vue";
import { useRouter } from "vue-router";
import logo from "@/assets/imgnest-mark.svg";
import { useUserAvatar } from "@/components/account/useUserAvatar";
import { menuIcons } from "@/integrations/vben/menuIcons";
import { workspaceMenus, type WorkspaceMenu } from "@/integrations/vben/navigation";
import {
  LOCALE_OPTIONS,
  localeMode,
  setLocaleMode,
  type LocaleMode,
} from "@/integrations/locale/useLocaleMode";
import { landingPath } from "@/router/landing";
import { useAuthStore } from "@/stores/auth";
import { useSiteStore } from "@/stores/site";
import { useSystemStore } from "@/stores/system";

/**
 * Marvis 外壳的漂浮侧栏：只负责呈现与发出意图，搜索/设置/退出/折叠由布局处理。
 * 语言切换是自包含的小菜单，直接写语言偏好。
 */
const props = defineProps<{ collapsed: boolean; collapsible?: boolean }>();
const emit = defineEmits<{
  navigate: [];
  "open-search": [];
  "open-settings": [];
  "toggle-collapse": [];
  logout: [];
}>();

const { t } = useI18n();
const router = useRouter();
const auth = useAuthStore();
const site = useSiteStore();
const system = useSystemStore();
const avatarUrl = useUserAvatar(computed(() => auth.user));
const user = computed(() => (auth.token ? auth.user : null));

interface Section {
  key: string;
  label?: string;
  items: WorkspaceMenu[];
}

/** 顶层菜单为一组；带子项的菜单（站点管理）展开成带标题的分组，自身不可点。 */
const sections = computed<Section[]>(() => {
  const menus = workspaceMenus(user.value?.role, site.galleryEnabled, system.updateAvailable);
  const result: Section[] = [{ key: "main", items: menus.filter((menu) => !menu.children) }];
  for (const menu of menus)
    if (menu.children) result.push({ key: menu.path, label: menu.name, items: menu.children });
  return result.filter((section) => section.items.length > 0);
});

const displayName = computed(
  () => user.value?.display_name || user.value?.username || t("common.account"),
);
const shortcut = /mac/i.test(navigator.userAgent) ? "⌘K" : "Ctrl K";

/** 站点名按显示宽度（中日韩字符算 2）逐级缩小字号；放不下时最多换两行。 */
const brandWidth = computed(() =>
  [...site.siteName].reduce((sum, char) => sum + (/[\u2E80-\uFFEF]/.test(char) ? 2 : 1), 0),
);
const brandSize = computed(() =>
  brandWidth.value <= 12 ? "is-lg" : brandWidth.value <= 18 ? "is-md" : "is-sm",
);

const localeOptions = computed<DropdownOption[]>(() =>
  LOCALE_OPTIONS.map((option) => ({
    key: option.value,
    label: option.label ?? t("shell.appearance.languageAuto"),
    props: { lang: option.lang, "aria-checked": localeMode.value === option.value },
    icon: () => (localeMode.value === option.value ? "✓" : ""),
  })),
);
function onLocaleSelect(key: string | number): void {
  void setLocaleMode(key as LocaleMode);
}

function goHome(): void {
  emit("navigate");
  void router.push(user.value ? landingPath(user.value.role) : "/gallery");
}
</script>

<template>
  <aside class="mv-sidebar" :class="{ 'is-collapsed': collapsed }">
    <div class="mv-brand-row">
      <button
        type="button"
        class="mv-brand"
        :aria-label="site.siteName"
        :title="site.siteName"
        @click="goHome"
      >
        <img class="mv-brand__mark" :src="logo" alt="" />
        <template v-if="!collapsed">
          <b class="mv-brand__name" :class="brandSize" data-testid="sidebar-brand-name">{{
            site.siteName
          }}</b>
          <span v-if="brandSize === 'is-lg'" class="mv-brand__badge">{{
            t("shell.brandBadge")
          }}</span>
        </template>
      </button>
      <button
        v-if="props.collapsible"
        type="button"
        class="mv-icon-btn mv-collapse-btn"
        data-testid="sidebar-collapse"
        :aria-label="t(collapsed ? 'shell.expandSidebar' : 'shell.collapseSidebar')"
        :title="t(collapsed ? 'shell.expandSidebar' : 'shell.collapseSidebar')"
        :aria-expanded="!collapsed"
        @click="emit('toggle-collapse')"
      >
        <PanelLeft class="mv-icon" />
      </button>
    </div>

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
            <span
              v-if="item.dot"
              class="mv-nav__dot"
              :class="{ 'is-collapsed': collapsed }"
              role="status"
              :aria-label="t('admin.settings.updateDot')"
              :title="t('admin.settings.updateDot')"
              data-testid="sidebar-update-dot"
            />
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
      <NDropdown
        trigger="click"
        :placement="collapsed ? 'right-end' : 'top'"
        :options="localeOptions"
        @select="onLocaleSelect"
      >
        <button
          type="button"
          class="mv-icon-btn"
          data-testid="sidebar-language"
          :aria-label="t('shell.appearance.language')"
          :title="t('shell.appearance.language')"
        >
          <Languages class="mv-icon" />
        </button>
      </NDropdown>
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
  /* 只有菜单区滚动：品牌、搜索和底部账户栏始终可见，矮屏幕上不会互相遮挡。 */
  overflow: hidden;
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
.mv-brand-row {
  display: flex;
  flex-shrink: 0;
  gap: 4px;
  align-items: flex-start;
  padding-bottom: 16px;
}
.mv-brand {
  display: flex;
  flex: 1;
  gap: 8px;
  align-items: center;
  min-width: 0;
  padding: 2px 0 0 10px;
  text-align: left;
}
.mv-collapse-btn {
  margin-top: -1px;
}
.mv-brand__mark {
  flex-shrink: 0;
  width: 24px;
  height: 24px;
  border-radius: 8px;
}
.mv-brand__name {
  display: -webkit-box;
  min-width: 0;
  overflow: hidden;
  font-size: 21px;
  font-weight: 800;
  line-height: 1.2;
  letter-spacing: -0.02em;
  overflow-wrap: anywhere;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}
.mv-brand__name.is-md {
  font-size: 17px;
}
.mv-brand__name.is-sm {
  font-size: 15px;
  letter-spacing: 0;
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
  flex-shrink: 0;
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
  margin-inline: -4px;
  padding-inline: 4px;
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-width: thin;
}
.mv-nav > * {
  flex-shrink: 0;
}
.mv-nav__label {
  padding: 20px 10px 6px;
  font-size: 11px;
  font-weight: 600;
  color: hsl(var(--muted-foreground));
  letter-spacing: 0.06em;
}
.mv-nav__item {
  position: relative;
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
.is-collapsed .mv-brand-row {
  flex-direction: column;
  gap: 8px;
  align-items: center;
}
.mv-nav__dot {
  flex: none;
  width: 8px;
  height: 8px;
  margin-left: auto;
  border-radius: 50%;
  background: hsl(var(--destructive));
}
.mv-nav__dot.is-collapsed {
  position: absolute;
  top: 6px;
  right: 10px;
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
