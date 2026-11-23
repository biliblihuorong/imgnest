<script setup lang="ts">
import { X } from "@vben/icons";
import { useI18n } from "@vben/locales";
import { computed, shallowRef, watch } from "vue";
import { useRouter } from "vue-router";
import { appConfig } from "@/app.config";
import { useAuthStore } from "@/stores/auth";
import MarvisDialog from "./MarvisDialog.vue";
import { settingsPanes } from "./settingsPanes";

/**
 * 设置弹窗（两套外壳共用）：左侧分类 + 右侧内容；分类在 settingsPanes.ts 里登记。
 * 只有一类时不显示左侧分类栏。配置了 appConfig.accountPath 时，登录用户在底部看到账号页入口。
 */
const show = defineModel<boolean>("show", { default: false });
const { t } = useI18n();
const router = useRouter();
const auth = useAuthStore();
const signedIn = computed(() => Boolean(auth.token && auth.user));
const accountPath = appConfig.accountPath;

const panes = computed(() =>
  signedIn.value ? settingsPanes : settingsPanes.filter((pane) => !pane.requiresAuth),
);
const single = computed(() => panes.value.length <= 1);
const first = () => panes.value[0]?.key ?? "";
const active = shallowRef(first());
const current = computed(() => panes.value.find((pane) => pane.key === active.value));
watch(panes, (list) => {
  if (!list.some((pane) => pane.key === active.value)) active.value = first();
});
watch(show, (open) => {
  if (open) active.value = first();
});

async function openAccount(): Promise<void> {
  show.value = false;
  await router.push(accountPath);
}
</script>

<template>
  <MarvisDialog v-model:show="show" :label="t('shell.settings.title')">
    <div class="mv-settings" :class="{ 'is-single': single }">
      <div
        v-if="!single"
        class="mv-settings__nav"
        role="tablist"
        :aria-label="t('shell.settings.title')"
      >
        <div class="mv-settings__label">{{ t("shell.settings.title") }}</div>
        <button
          v-for="pane in panes"
          :key="pane.key"
          type="button"
          role="tab"
          class="mv-settings__tab"
          :data-testid="`settings-tab-${pane.key}`"
          :aria-selected="active === pane.key"
          @click="active = pane.key"
        >
          <component :is="pane.icon" class="mv-settings__icon" />{{ t(pane.label) }}
        </button>
      </div>
      <div class="mv-settings__content">
        <div class="mv-settings__head">
          <span>{{ current ? t(current.label) : "" }}</span>
          <button
            type="button"
            class="mv-settings__close"
            data-testid="settings-close"
            :aria-label="t('shell.settings.close')"
            @click="show = false"
          >
            <X class="mv-settings__icon" />
          </button>
        </div>
        <div class="mv-settings__body" :role="single ? undefined : 'tabpanel'">
          <component :is="current.component" v-if="current" />
        </div>
        <div v-if="accountPath && signedIn" class="mv-settings__foot">
          <span>{{ t("shell.settings.accountHint") }}</span>
          <button
            type="button"
            class="mv-settings__link"
            data-testid="settings-account-link"
            @click="openAccount"
          >
            {{ t("shell.settings.accountLink") }}
          </button>
        </div>
      </div>
    </div>
  </MarvisDialog>
</template>

<style scoped>
.mv-settings {
  display: flex;
  width: min(720px, 92vw);
  height: min(560px, 86vh);
  overflow: hidden;
  color: hsl(var(--foreground));
  background: hsl(var(--card));
  border-radius: 18px;
  box-shadow: 0 32px 90px rgb(0 0 0 / 28%);
}
.mv-settings.is-single {
  width: min(560px, 92vw);
  height: auto;
  max-height: min(640px, 86vh);
}
.mv-settings__nav {
  display: flex;
  flex-shrink: 0;
  flex-direction: column;
  gap: 1px;
  width: 198px;
  padding: 18px 10px;
  background: hsl(var(--background-deep));
  border-right: 1px solid hsl(var(--border));
}
.mv-settings__label {
  padding: 2px 10px 12px;
  font-size: 13px;
  font-weight: 700;
}
.mv-settings__tab {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 8px 10px;
  font-size: 13.5px;
  font-weight: 500;
  text-align: left;
  white-space: nowrap;
  border-radius: 8px;
}
.mv-settings__tab:hover {
  background: hsl(var(--muted));
}
.mv-settings__tab[aria-selected="true"] {
  font-weight: 700;
  background: hsl(var(--card));
  box-shadow: var(--mv-shadow-sm);
}
.mv-settings__icon {
  flex-shrink: 0;
  width: 16px;
  height: 16px;
}
.mv-settings__content {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
}
.mv-settings__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 18px 26px 12px;
  font-size: 16px;
  font-weight: 800;
}
.mv-settings__close {
  display: grid;
  place-items: center;
  width: 29px;
  height: 29px;
  color: hsl(var(--muted-foreground));
  border-radius: 8px;
}
.mv-settings__close:hover {
  background: hsl(var(--muted));
}
.mv-settings__body {
  flex: 1;
  min-height: 0;
  padding: 2px 26px 26px;
  overflow-y: auto;
}
.mv-settings__foot {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
  justify-content: space-between;
  padding: 14px 26px;
  font-size: 13px;
  color: hsl(var(--muted-foreground));
  border-top: 1px solid hsl(var(--border));
}
.mv-settings__link {
  padding: 7px 14px;
  font-size: 13px;
  font-weight: 600;
  color: hsl(var(--foreground));
  border: 1px solid hsl(var(--border));
  border-radius: 10px;
}
.mv-settings__link:hover {
  background: hsl(var(--muted));
}

@media (width <= 768px) {
  .mv-settings,
  .mv-settings.is-single {
    flex-direction: column;
    width: 100vw;
    height: 100dvh;
    max-height: none;
    border-radius: 0;
  }
  .mv-settings__nav {
    flex-direction: row;
    gap: 6px;
    width: 100%;
    padding: 12px 10px;
    overflow-x: auto;
    border-right: none;
    border-bottom: 1px solid hsl(var(--border));
  }
  .mv-settings__label {
    display: none;
  }
}
</style>
