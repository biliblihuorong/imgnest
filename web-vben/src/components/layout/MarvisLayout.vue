<script setup lang="ts">
import { Menu } from "@vben/icons";
import { useI18n } from "@vben/locales";
import { preferences } from "@vben/preferences";
import { computed, onBeforeUnmount, shallowRef, watch } from "vue";
import { useRouter } from "vue-router";
import { useSiteStore } from "@/stores/site";
import MarvisCommandPalette from "./marvis/MarvisCommandPalette.vue";
import MarvisSettingsModal from "./marvis/MarvisSettingsModal.vue";
import MarvisSidebar from "./marvis/MarvisSidebar.vue";
import SidebarAlbums from "./marvis/SidebarAlbums.vue";
import { useViewportBelow } from "./marvis/useNarrowViewport";
import { useLogout } from "./useLogout";

/** Marvis 外壳：灰底 + 漂浮侧栏 + 路由内容；窄屏侧栏变抽屉。 */
const { t } = useI18n();
const router = useRouter();
const site = useSiteStore();
const { logout } = useLogout();
void site.ensureLoaded();

const narrow = useViewportBelow(769);
const drawerOpen = shallowRef(false);
const paletteOpen = shallowRef(false);
const settingsOpen = shallowRef(false);
const collapsed = computed(() => !narrow.value && preferences.sidebar.collapsed);

watch(narrow, (value) => {
  if (!value) drawerOpen.value = false;
});
const stopAfterEach = router.afterEach(() => {
  drawerOpen.value = false;
});
onBeforeUnmount(stopAfterEach);
</script>

<template>
  <div class="mv-app" :class="{ 'is-narrow': narrow }" data-testid="marvis-layout">
    <button
      v-if="narrow"
      type="button"
      class="mv-menu-btn"
      data-testid="marvis-menu-button"
      :aria-label="t('shell.openMenu')"
      :aria-expanded="drawerOpen"
      @click="drawerOpen = true"
    >
      <Menu class="mv-menu-btn__icon" />
    </button>
    <div
      v-if="narrow && drawerOpen"
      class="mv-backdrop"
      data-testid="marvis-backdrop"
      @click="drawerOpen = false"
    />
    <MarvisSidebar
      data-testid="marvis-sidebar"
      :class="{ 'is-open': drawerOpen }"
      :collapsed="collapsed"
      @navigate="drawerOpen = false"
      @open-search="((drawerOpen = false), (paletteOpen = true))"
      @open-settings="((drawerOpen = false), (settingsOpen = true))"
      @logout="logout"
    >
      <template #albums><SidebarAlbums @navigate="drawerOpen = false" /></template>
    </MarvisSidebar>
    <MarvisCommandPalette v-model:show="paletteOpen" />
    <MarvisSettingsModal v-model:show="settingsOpen" />
    <main class="mv-main">
      <RouterView />
    </main>
  </div>
</template>

<style scoped>
.mv-app {
  display: flex;
  width: 100%;
  height: 100%;
  overflow: hidden;
  color: hsl(var(--foreground));
  background: hsl(var(--background-deep));
}
.mv-main {
  flex: 1;
  min-width: 0;
  height: 100%;
  overflow: auto;
}
.mv-menu-btn {
  position: fixed;
  top: 12px;
  left: 12px;
  z-index: 44;
  display: grid;
  place-items: center;
  width: 38px;
  height: 38px;
  background: hsl(var(--card));
  border-radius: 12px;
  box-shadow: var(--mv-shadow-md);
}
.mv-menu-btn__icon {
  width: 18px;
  height: 18px;
}
.mv-backdrop {
  position: fixed;
  inset: 0;
  z-index: 45;
  background: hsl(var(--overlay));
}
.is-narrow :deep(.mv-sidebar) {
  position: fixed;
  top: 0;
  bottom: 0;
  left: 0;
  z-index: 46;
  width: min(280px, 84vw);
  margin: 10px;
  transform: translateX(calc(-100% - 24px));
  transition: transform 0.34s var(--mv-ease);
}
.is-narrow :deep(.mv-sidebar.is-open) {
  transform: none;
}
.is-narrow .mv-main {
  padding-top: 56px;
}
</style>
