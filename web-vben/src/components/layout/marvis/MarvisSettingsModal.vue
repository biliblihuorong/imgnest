<script setup lang="ts">
import { LockKeyhole, Palette, UserRoundPen, X } from "@vben/icons";
import { useI18n } from "@vben/locales";
import { shallowRef, watch, type Component } from "vue";
import { useRouter } from "vue-router";
import ChangePasswordCard from "@/components/account/ChangePasswordCard.vue";
import ProfileCard from "@/components/account/ProfileCard.vue";
import AppearancePane from "./AppearancePane.vue";
import MarvisDialog from "./MarvisDialog.vue";

/** 设置弹窗：左侧分类 + 右侧内容。Token 明文流程不搬进来，只给入口。 */
const show = defineModel<boolean>("show", { default: false });
const { t } = useI18n();
const router = useRouter();

type Pane = "appearance" | "account" | "tokens";
const panes: { key: Pane; label: string; icon: Component }[] = [
  { key: "appearance", label: "shell.settings.appearance", icon: Palette },
  { key: "account", label: "shell.settings.account", icon: UserRoundPen },
  { key: "tokens", label: "shell.settings.tokens", icon: LockKeyhole },
];
const active = shallowRef<Pane>("appearance");
watch(show, (open) => {
  if (open) active.value = "appearance";
});

async function openTokens(): Promise<void> {
  show.value = false;
  await router.push("/tokens");
}
</script>

<template>
  <MarvisDialog v-model:show="show" :label="t('shell.settings.title')">
    <div class="mv-settings">
      <div class="mv-settings__nav" role="tablist" :aria-label="t('shell.settings.title')">
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
          <span>{{ t(panes.find((pane) => pane.key === active)!.label) }}</span>
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
        <div class="mv-settings__body" role="tabpanel">
          <AppearancePane v-if="active === 'appearance'" />
          <template v-else-if="active === 'account'">
            <ProfileCard />
            <h3 class="mv-settings__section">{{ t("account.securityTitle") }}</h3>
            <p class="mv-settings__hint">{{ t("account.securityHint") }}</p>
            <ChangePasswordCard />
          </template>
          <template v-else>
            <p class="mv-settings__hint">{{ t("shell.settings.tokensHint") }}</p>
            <button
              type="button"
              class="mv-settings__link"
              data-testid="settings-tokens-link"
              @click="openTokens"
            >
              {{ t("shell.settings.tokensLink") }}
            </button>
          </template>
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
.mv-settings__section {
  margin: 24px 0 4px;
  font-size: 14px;
  font-weight: 700;
}
.mv-settings__hint {
  margin: 0 0 16px;
  font-size: 13px;
  color: hsl(var(--muted-foreground));
}
.mv-settings__link {
  padding: 7px 14px;
  font-size: 13px;
  font-weight: 600;
  border: 1px solid hsl(var(--border));
  border-radius: 10px;
}
.mv-settings__link:hover {
  background: hsl(var(--muted));
}

@media (width <= 768px) {
  .mv-settings {
    flex-direction: column;
    width: 100vw;
    height: 100dvh;
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
