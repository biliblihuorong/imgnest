<script setup lang="ts">
import { CornerDownLeft, Search } from "@vben/icons";
import { useI18n } from "@vben/locales";
import { computed, onBeforeUnmount, onMounted, shallowRef, watch } from "vue";
import { useRouter } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import { leafMenus, type MenuItem } from "../navigation";
import MarvisDialog from "./MarvisDialog.vue";

/**
 * Ctrl/⌘+K 命令面板：按名称筛选当前账号可见的页面并跳转。
 * 要加业务搜索（例如搜订单号），在 pages 之前插一组结果即可。
 */
const show = defineModel<boolean>("show", { default: false });
const { t } = useI18n();
const router = useRouter();
const auth = useAuthStore();

const raw = shallowRef("");
const selected = shallowRef(0);
const signedIn = computed(() => Boolean(auth.token && auth.user));
const pages = computed<MenuItem[]>(() => {
  const leaves = leafMenus(auth.user);
  const needle = raw.value.trim().toLowerCase();
  return needle
    ? leaves.filter((menu) => `${t(menu.name)} ${menu.path}`.toLowerCase().includes(needle))
    : leaves;
});

watch(show, (open) => {
  if (open) raw.value = "";
});
watch(pages, () => {
  selected.value = 0;
});

function onShortcut(event: KeyboardEvent): void {
  if (!(event.ctrlKey || event.metaKey) || event.altKey || event.shiftKey) return;
  if (event.isComposing || event.key.toLowerCase() !== "k" || !signedIn.value) return;
  // 已有别的模态层时不在它上面/下面再开一层。
  if (!show.value && document.querySelector('[aria-modal="true"], .n-modal')) return;
  event.preventDefault();
  show.value = !show.value;
}
onMounted(() => window.addEventListener("keydown", onShortcut));
onBeforeUnmount(() => window.removeEventListener("keydown", onShortcut));
// 会话结束时立即收起，避免在登录页上残留。
watch(signedIn, (value) => {
  if (!value) show.value = false;
});

async function open(menu: MenuItem | undefined): Promise<void> {
  if (!menu) return;
  show.value = false;
  await router.push(menu.path);
}
function onKey(event: KeyboardEvent): void {
  if (event.isComposing || event.keyCode === 229) return;
  const count = pages.value.length;
  if (["ArrowDown", "ArrowUp"].includes(event.key) && count > 0) {
    event.preventDefault();
    selected.value = (selected.value + (event.key === "ArrowDown" ? 1 : count - 1)) % count;
  } else if (event.key === "Enter") {
    event.preventDefault();
    void open(pages.value[selected.value]);
  }
}
</script>

<template>
  <MarvisDialog v-model:show="show" :label="t('shell.search')" align="top">
    <div class="mv-palette">
      <div class="mv-palette__input">
        <Search class="mv-palette__icon" />
        <input
          v-model="raw"
          data-testid="palette-input"
          data-autofocus
          type="text"
          autocomplete="off"
          spellcheck="false"
          :placeholder="t('shell.palette.placeholder')"
          :aria-label="t('shell.palette.placeholder')"
          @keydown="onKey"
        />
        <kbd>Esc</kbd>
      </div>
      <div class="mv-palette__list">
        <template v-if="pages.length > 0">
          <div class="mv-palette__heading">{{ t("shell.palette.pages") }}</div>
          <button
            v-for="(menu, index) in pages"
            :key="menu.path"
            type="button"
            class="mv-palette__row"
            :class="{ 'is-selected': index === selected }"
            data-testid="palette-page"
            @mouseenter="selected = index"
            @click="open(menu)"
          >
            <component :is="menu.icon" v-if="menu.icon" class="mv-palette__icon" />
            <span>{{ t(menu.name) }}</span>
            <CornerDownLeft v-if="index === selected" class="mv-palette__icon mv-palette__enter" />
          </button>
        </template>
        <div v-else class="mv-palette__empty">{{ t("shell.palette.empty") }}</div>
      </div>
    </div>
  </MarvisDialog>
</template>

<style scoped>
.mv-palette {
  width: min(560px, 92vw);
  overflow: hidden;
  color: hsl(var(--foreground));
  background: hsl(var(--popover));
  border-radius: 16px;
  box-shadow: 0 32px 90px rgb(0 0 0 / 28%);
}
.mv-palette__input {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 14px 16px;
  color: hsl(var(--muted-foreground));
  border-bottom: 1px solid hsl(var(--border));
}
.mv-palette__input input {
  flex: 1;
  min-width: 0;
  font-size: 15px;
  color: hsl(var(--foreground));
  outline: none;
  background: none;
  border: none;
}
.mv-palette__icon {
  flex-shrink: 0;
  width: 16px;
  height: 16px;
}
.mv-palette kbd {
  flex-shrink: 0;
  padding: 1px 6px;
  margin-left: auto;
  font-family: inherit;
  font-size: 10px;
  color: hsl(var(--muted-foreground));
  border: 1px solid hsl(var(--border));
  border-radius: 5px;
}
.mv-palette__list {
  max-height: min(360px, 60vh);
  padding: 6px;
  overflow-y: auto;
}
.mv-palette__heading {
  padding: 8px 10px 4px;
  font-size: 11px;
  font-weight: 600;
  color: hsl(var(--muted-foreground));
  letter-spacing: 0.06em;
}
.mv-palette__row {
  display: flex;
  gap: 10px;
  align-items: center;
  width: 100%;
  padding: 8px 10px;
  font-size: 13.5px;
  text-align: left;
  border-radius: 8px;
}
.mv-palette__row span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mv-palette__row:hover,
.mv-palette__row.is-selected {
  background: hsl(var(--muted));
}
.mv-palette__enter {
  margin-left: auto;
  color: hsl(var(--muted-foreground));
}
.mv-palette__empty {
  padding: 18px 10px;
  font-size: 13px;
  color: hsl(var(--muted-foreground));
  text-align: center;
}
</style>
