<script setup lang="ts">
// 系统设置外壳：左侧（窄屏为顶部）二级导航，右侧渲染当前设置页。
import { Page } from "@vben/common-ui";
import { useI18n } from "@vben/locales";
import { onMounted } from "vue";
import { useSystemStore } from "@/stores/system";
import { SETTINGS_SECTIONS } from "./settings/sections";

const { t } = useI18n();
const system = useSystemStore();
onMounted(() => {
  void system.load();
});
</script>

<template>
  <Page
    :title="t('admin.settings.title')"
    :description="t('admin.settings.description')"
    class="settings-view min-w-0"
    content-class="min-w-0"
  >
    <div class="settings-layout">
      <nav class="settings-nav" :aria-label="t('admin.settings.title')">
        <RouterLink
          v-for="section in SETTINGS_SECTIONS"
          :key="section.key"
          :to="{ name: section.name }"
          class="settings-nav__item"
          :data-section="section.key"
        >
          <span class="settings-nav__title"
            >{{ t(`admin.settings.sections.${section.key}.title`)
            }}<span
              v-if="section.key === 'about' && system.updateAvailable"
              class="settings-nav__dot"
              role="status"
              :aria-label="t('admin.settings.updateDot')"
              :title="t('admin.settings.updateDot')"
              data-testid="settings-update-dot"
          /></span>
          <span class="settings-nav__description">{{
            t(`admin.settings.sections.${section.key}.description`)
          }}</span>
        </RouterLink>
      </nav>
      <div class="settings-content">
        <RouterView />
      </div>
    </div>
  </Page>
</template>

<style scoped>
.settings-layout {
  display: grid;
  grid-template-columns: 220px minmax(0, 1fr);
  gap: 20px;
  align-items: start;
}

.settings-nav {
  display: flex;
  flex-direction: column;
  gap: 4px;
  position: sticky;
  top: 16px;
}

.settings-nav__item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 10px 12px;
  border-radius: 8px;
  color: hsl(var(--foreground));
  text-decoration: none;
}

.settings-nav__item:hover {
  background: hsl(var(--accent));
}

.settings-nav__item.router-link-active {
  background: hsl(var(--primary) / 10%);
  color: hsl(var(--primary));
}

.settings-nav__title {
  font-weight: 600;
}

.settings-nav__dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  margin-left: 6px;
  vertical-align: middle;
  border-radius: 50%;
  background: hsl(var(--destructive));
}

.settings-nav__description {
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}

.settings-content {
  min-width: 0;
}

@media (max-width: 768px) {
  .settings-layout {
    grid-template-columns: minmax(0, 1fr);
  }

  .settings-nav {
    position: static;
    flex-direction: row;
    overflow-x: auto;
    padding-bottom: 4px;
  }

  .settings-nav__item {
    flex: 0 0 auto;
  }

  .settings-nav__description {
    display: none;
  }
}
</style>
