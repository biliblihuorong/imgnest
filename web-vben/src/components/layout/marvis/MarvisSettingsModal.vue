<script setup lang="ts">
import { X } from "@vben/icons";
import { useI18n } from "@vben/locales";
import { computed } from "vue";
import { useRouter } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import AppearancePane from "./AppearancePane.vue";
import MarvisDialog from "./MarvisDialog.vue";

/**
 * 设置弹窗：两套外壳共用，只管外观与语言。
 * 资料、密码与 API Token 都在账户页，这里只给登录用户一个入口。
 */
const show = defineModel<boolean>("show", { default: false });
const { t } = useI18n();
const router = useRouter();
const auth = useAuthStore();
const signedIn = computed(() => Boolean(auth.token && auth.user));

async function openAccount(): Promise<void> {
  show.value = false;
  await router.push("/account");
}
</script>

<template>
  <MarvisDialog v-model:show="show" :label="t('shell.settings.title')">
    <div class="mv-settings">
      <div class="mv-settings__head">
        <span>{{ t("shell.settings.title") }}</span>
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
      <div class="mv-settings__body">
        <AppearancePane />
      </div>
      <div v-if="signedIn" class="mv-settings__foot">
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
  </MarvisDialog>
</template>

<style scoped>
.mv-settings {
  display: flex;
  flex-direction: column;
  width: min(560px, 92vw);
  max-height: min(640px, 86vh);
  overflow: hidden;
  color: hsl(var(--foreground));
  background: hsl(var(--card));
  border-radius: 18px;
  box-shadow: 0 32px 90px rgb(0 0 0 / 28%);
}
.mv-settings__icon {
  flex-shrink: 0;
  width: 16px;
  height: 16px;
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
  padding: 2px 26px 20px;
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
  .mv-settings {
    width: 100vw;
    max-height: none;
    height: 100dvh;
    border-radius: 0;
  }
}
</style>
