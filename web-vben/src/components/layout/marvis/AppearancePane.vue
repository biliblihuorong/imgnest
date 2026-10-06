<script lang="ts">
import { SHELL_DEFAULTS } from "@/integrations/shell/useShell";

/** 强调色预设：两套外壳各自的默认色在前，其后是三个备选。 */
export const ACCENT_PRESETS = [
  SHELL_DEFAULTS.marvis.colorPrimary,
  SHELL_DEFAULTS.classic.colorPrimary,
  "hsl(160 84% 34%)",
  "hsl(18 76% 53%)",
  "hsl(317 54% 49%)",
] as const;
/** 圆角预设：小 / 中 / 大。 */
export const RADIUS_PRESETS = ["0.25", "0.5", "0.75"] as const;
</script>

<script setup lang="ts">
import { loadLocaleMessages, useI18n } from "@vben/locales";
import { preferences, updatePreferences } from "@vben/preferences";
import { resetAppearance, setShell, shell, type Shell } from "@/integrations/shell/useShell";

/**
 * 外观设置。除「布局」外都直接读写 Vben 偏好（已持久化并同步给 Naive UI），
 * 所以两套外壳共用同一份深浅色、强调色、圆角与侧栏折叠状态。
 */
const { t } = useI18n();

const layouts: { value: Shell; name: string; hint: string }[] = [
  {
    value: "marvis",
    name: "shell.appearance.layoutMarvis",
    hint: "shell.appearance.layoutMarvisHint",
  },
  {
    value: "classic",
    name: "shell.appearance.layoutClassic",
    hint: "shell.appearance.layoutClassicHint",
  },
];
const modes = [
  { value: "light", label: "shell.appearance.modeLight" },
  { value: "dark", label: "shell.appearance.modeDark" },
  { value: "auto", label: "shell.appearance.modeAuto" },
] as const;
// 语言名用各自的写法，不随界面语言翻译。
const locales = [
  { value: "zh-CN", label: "简体中文" },
  { value: "en-US", label: "English" },
] as const;

/** 与经典布局顶栏的语言切换同一套写法。 */
async function setLocale(locale: (typeof locales)[number]["value"]): Promise<void> {
  updatePreferences({ app: { locale } });
  await loadLocaleMessages(locale);
}

const radiusLabels = [
  "shell.appearance.radiusSmall",
  "shell.appearance.radiusMedium",
  "shell.appearance.radiusLarge",
];
</script>

<template>
  <div class="mv-appearance">
    <div class="mv-appearance__layouts" role="group" :aria-label="t('shell.appearance.layout')">
      <button
        v-for="layout in layouts"
        :key="layout.value"
        type="button"
        class="mv-appearance__layout"
        :class="{ 'is-on': shell === layout.value }"
        :data-testid="`layout-${layout.value}`"
        :aria-pressed="shell === layout.value"
        @click="setShell(layout.value)"
      >
        <span class="mv-appearance__preview" :class="`is-${layout.value}`" aria-hidden="true">
          <i /><i />
        </span>
        <b>{{ t(layout.name) }}</b>
        <small>{{ t(layout.hint) }}</small>
      </button>
    </div>

    <div class="mv-appearance__row">
      <span>{{ t("shell.appearance.mode") }}</span>
      <div class="mv-seg" role="group" :aria-label="t('shell.appearance.mode')">
        <button
          v-for="mode in modes"
          :key="mode.value"
          type="button"
          :data-testid="`mode-${mode.value}`"
          :aria-pressed="preferences.theme.mode === mode.value"
          @click="updatePreferences({ theme: { mode: mode.value } })"
        >
          {{ t(mode.label) }}
        </button>
      </div>
    </div>

    <div class="mv-appearance__row">
      <span>{{ t("shell.appearance.language") }}</span>
      <div class="mv-seg" role="group" :aria-label="t('shell.appearance.language')">
        <button
          v-for="locale in locales"
          :key="locale.value"
          type="button"
          :lang="locale.value"
          :data-testid="`locale-${locale.value}`"
          :aria-pressed="preferences.app.locale === locale.value"
          @click="setLocale(locale.value)"
        >
          {{ locale.label }}
        </button>
      </div>
    </div>

    <div class="mv-appearance__row">
      <span>{{ t("shell.appearance.accent") }}</span>
      <div class="mv-appearance__swatches" role="group" :aria-label="t('shell.appearance.accent')">
        <button
          v-for="(color, index) in ACCENT_PRESETS"
          :key="color"
          type="button"
          class="mv-appearance__swatch"
          :style="{ background: color }"
          :data-testid="`accent-${index}`"
          :aria-pressed="preferences.theme.colorPrimary === color"
          :aria-label="t('shell.appearance.accentOption', { index: index + 1 })"
          @click="updatePreferences({ theme: { colorPrimary: color } })"
        />
      </div>
    </div>

    <div class="mv-appearance__row">
      <span>{{ t("shell.appearance.radius") }}</span>
      <div class="mv-seg" role="group" :aria-label="t('shell.appearance.radius')">
        <button
          v-for="(radius, index) in RADIUS_PRESETS"
          :key="radius"
          type="button"
          :data-testid="`radius-${index}`"
          :aria-pressed="preferences.theme.radius === radius"
          @click="updatePreferences({ theme: { radius } })"
        >
          {{ t(radiusLabels[index]!) }}
        </button>
      </div>
    </div>

    <div class="mv-appearance__row">
      <span>
        {{ t("shell.appearance.compactSidebar") }}
        <small>{{ t("shell.appearance.compactSidebarHint") }}</small>
      </span>
      <button
        type="button"
        class="mv-switch"
        role="switch"
        data-testid="compact-sidebar"
        :aria-checked="preferences.sidebar.collapsed"
        :aria-label="t('shell.appearance.compactSidebar')"
        @click="updatePreferences({ sidebar: { collapsed: !preferences.sidebar.collapsed } })"
      />
    </div>

    <div class="mv-appearance__row">
      <span>
        {{ t("shell.appearance.reset") }}
        <small>{{ t("shell.appearance.resetHint") }}</small>
      </span>
      <button
        type="button"
        class="mv-appearance__reset"
        data-testid="appearance-reset"
        @click="resetAppearance"
      >
        {{ t("shell.appearance.resetAction") }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.mv-appearance__layouts {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 10px;
  padding: 6px 0 14px;
  border-bottom: 1px solid hsl(var(--border));
}
.mv-appearance__layout {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px;
  text-align: left;
  border: 1px solid hsl(var(--border));
  border-radius: 12px;
}
.mv-appearance__layout.is-on {
  border-color: hsl(var(--foreground));
}
.mv-appearance__layout b {
  margin-top: 6px;
  font-size: 13.5px;
}
.mv-appearance__layout small,
.mv-appearance__row small {
  display: block;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}
.mv-appearance__preview {
  display: flex;
  gap: 5px;
  height: 58px;
  padding: 6px;
  background: hsl(var(--background-deep));
  border-radius: 8px;
}
.mv-appearance__preview i {
  display: block;
  background: hsl(var(--card));
  border-radius: 5px;
  box-shadow: var(--mv-shadow-sm, 0 1px 2px rgb(0 0 0 / 6%));
}
.mv-appearance__preview.is-marvis i:first-child {
  width: 26%;
}
.mv-appearance__preview.is-marvis i:last-child {
  flex: 1;
  background: none;
  box-shadow: none;
}
.mv-appearance__preview.is-classic {
  flex-direction: column;
}
.mv-appearance__preview.is-classic i:first-child {
  height: 26%;
}
.mv-appearance__preview.is-classic i:last-child {
  flex: 1;
}
.mv-appearance__row {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
  justify-content: space-between;
  padding: 12px 0;
  font-size: 14px;
}
.mv-seg {
  display: flex;
  padding: 3px;
  background: hsl(var(--muted));
  border-radius: 9px;
}
.mv-seg button {
  padding: 4px 12px;
  font-size: 12.5px;
  font-weight: 500;
  color: hsl(var(--muted-foreground));
  border-radius: 7px;
}
.mv-seg button[aria-pressed="true"] {
  font-weight: 700;
  color: hsl(var(--foreground));
  background: hsl(var(--card));
  box-shadow: var(--mv-shadow-sm, 0 1px 2px rgb(0 0 0 / 6%));
}
.mv-appearance__swatches {
  display: flex;
  gap: 8px;
}
.mv-appearance__swatch {
  width: 24px;
  height: 24px;
  border: 2px solid hsl(var(--card));
  border-radius: 50%;
  box-shadow: 0 0 0 1px hsl(var(--border));
}
.mv-appearance__swatch[aria-pressed="true"] {
  box-shadow: 0 0 0 2px hsl(var(--foreground));
}
.mv-switch {
  position: relative;
  flex-shrink: 0;
  width: 39px;
  height: 22px;
  background: hsl(var(--input));
  border-radius: 99px;
  transition: background 0.25s var(--mv-ease, ease);
}
.mv-switch::after {
  position: absolute;
  top: 2.5px;
  left: 3px;
  width: 17px;
  height: 17px;
  content: "";
  background: #fff;
  border-radius: 50%;
  box-shadow: 0 1px 3px rgb(0 0 0 / 22%);
  transition: transform 0.25s var(--mv-ease, ease);
}
.mv-switch[aria-checked="true"] {
  background: hsl(var(--foreground));
}
.mv-switch[aria-checked="true"]::after {
  background: hsl(var(--background));
  transform: translateX(16px);
}
.mv-appearance__reset {
  padding: 5px 14px;
  font-size: 12.5px;
  font-weight: 600;
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
}
.mv-appearance__reset:hover {
  background: hsl(var(--muted));
}
</style>
