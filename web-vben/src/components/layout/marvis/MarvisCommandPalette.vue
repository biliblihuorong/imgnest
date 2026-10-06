<script setup lang="ts">
import { Search } from "@vben/icons";
import { useI18n } from "@vben/locales";
import {
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  shallowRef,
  useTemplateRef,
  watch,
} from "vue";
import { useRouter } from "vue-router";
import { defaultTimezone } from "@/components/images/useImageQueryState";
import {
  useSearchSuggestions,
  type SearchSuggestion,
} from "@/components/images/useSearchSuggestions";
import { menuIcons } from "@/integrations/vben/menuIcons";
import { workspaceMenus, type WorkspaceMenu } from "@/integrations/vben/navigation";
import { useAuthStore } from "@/stores/auth";
import { useSiteStore } from "@/stores/site";

/**
 * Ctrl/⌘+K 全局检索。只收集输入并跳转到 /images，由统一搜索解析与校验；
 * 这里不实现搜索语法。补全建议复用图片页的 useSearchSuggestions。
 */
const show = defineModel<boolean>("show", { default: false });
const { t } = useI18n();
const router = useRouter();
const auth = useAuthStore();
const site = useSiteStore();
const input = useTemplateRef<HTMLInputElement>("input");
const dialog = useTemplateRef<HTMLElement>("dialog");

const raw = shallowRef("");
const caret = shallowRef(0);
const composing = shallowRef(false);
const focused = shallowRef(false);
const suggestions = useSearchSuggestions({
  raw,
  caret,
  composing,
  focused,
  scopeAlbumId: shallowRef(undefined),
});

const signedIn = computed(() => Boolean(auth.token && auth.user));
const pages = computed<WorkspaceMenu[]>(() => {
  const leaves = workspaceMenus(auth.user?.role, site.galleryEnabled).flatMap(
    (menu) => menu.children ?? [menu],
  );
  const needle = raw.value.trim().toLowerCase();
  return needle ? leaves.filter((menu) => t(menu.name).toLowerCase().includes(needle)) : leaves;
});

let returnTo: HTMLElement | null = null;
watch(show, async (open) => {
  if (open) {
    returnTo = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    raw.value = "";
    caret.value = 0;
    composing.value = false;
    await nextTick();
    input.value?.focus();
    return;
  }
  focused.value = false;
  const target = returnTo;
  returnTo = null;
  await nextTick();
  if (target?.isConnected) target.focus();
});

function onShortcut(event: KeyboardEvent): void {
  if (!(event.ctrlKey || event.metaKey) || event.altKey || event.shiftKey) return;
  if (event.key.toLowerCase() !== "k" || !signedIn.value) return;
  event.preventDefault();
  show.value = !show.value;
}
onMounted(() => window.addEventListener("keydown", onShortcut));
onBeforeUnmount(() => window.removeEventListener("keydown", onShortcut));
// 会话结束时立即收起，避免在登录页上残留。
watch(signedIn, (value) => {
  if (!value) show.value = false;
});

/** 弹窗内循环 Tab 焦点，不让焦点落到被遮住的页面上。 */
function trapTab(event: KeyboardEvent): void {
  const focusable = dialog.value?.querySelectorAll<HTMLElement>("input, button");
  if (!focusable || focusable.length === 0) return;
  const first = focusable[0]!;
  const last = focusable[focusable.length - 1]!;
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault();
    last.focus();
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first.focus();
  }
}

function capture(): void {
  caret.value = input.value?.selectionStart ?? raw.value.length;
}
function update(event: Event): void {
  raw.value = (event.target as HTMLInputElement).value;
  if (!composing.value) capture();
}
function compositionEnd(event: CompositionEvent): void {
  composing.value = false;
  update(event);
}
async function choose(suggestion: SearchSuggestion): Promise<void> {
  raw.value =
    raw.value.slice(0, suggestion.span.start) +
    suggestion.insert +
    raw.value.slice(suggestion.span.end);
  const position = suggestion.span.start + suggestion.insert.length;
  suggestions.close();
  await nextTick();
  input.value?.focus();
  input.value?.setSelectionRange(position, position);
  capture();
}
async function submit(): Promise<void> {
  const q = raw.value.trim();
  show.value = false;
  await router.push(
    q
      ? { path: "/images", query: { qv: "1", q, tz: defaultTimezone(), page: "1", size: "20" } }
      : { path: "/images" },
  );
}
async function open(menu: WorkspaceMenu): Promise<void> {
  show.value = false;
  await router.push(menu.path);
}
function onKey(event: KeyboardEvent): void {
  if (composing.value || event.isComposing || event.keyCode === 229) return;
  if (event.key === "Escape") {
    event.preventDefault();
    event.stopPropagation();
    if (suggestions.visible.value) suggestions.close();
    else show.value = false;
    return;
  }
  if (["ArrowDown", "ArrowUp"].includes(event.key) && suggestions.items.value.length > 0) {
    event.preventDefault();
    suggestions.move(event.key === "ArrowDown" ? 1 : -1);
    return;
  }
  if (event.key === "Enter") {
    event.preventDefault();
    const selected = suggestions.visible.value
      ? suggestions.items.value[suggestions.selected.value]
      : undefined;
    if (selected) void choose(selected);
    else void submit();
  }
}
</script>

<template>
  <Teleport to="body">
    <div v-if="show" class="mv-palette-overlay" @mousedown.self="show = false">
      <div
        ref="dialog"
        class="mv-palette"
        role="dialog"
        aria-modal="true"
        :aria-label="t('shell.search')"
        @keydown.tab="trapTab"
        @keydown.esc="show = false"
      >
        <div class="mv-palette__input">
          <Search class="mv-palette__icon" />
          <input
            ref="input"
            data-testid="palette-input"
            type="text"
            autocomplete="off"
            spellcheck="false"
            :value="raw"
            :placeholder="t('shell.palette.placeholder')"
            :aria-label="t('shell.palette.placeholder')"
            @input="update"
            @keydown="onKey"
            @keyup="capture"
            @click="capture"
            @focus="focused = true"
            @blur="focused = false"
            @compositionstart="composing = true"
            @compositionend="compositionEnd"
          />
          <kbd>Esc</kbd>
        </div>
        <div class="mv-palette__list">
          <template v-if="suggestions.visible.value && suggestions.items.value.length > 0">
            <div class="mv-palette__heading">{{ t("shell.palette.conditions") }}</div>
            <button
              v-for="(item, index) in suggestions.items.value"
              :key="item.label"
              type="button"
              class="mv-palette__row"
              :class="{ 'is-selected': index === suggestions.selected.value }"
              data-testid="palette-suggestion"
              @mousedown.prevent
              @click="choose(item)"
            >
              <code>{{ item.label }}</code>
            </button>
          </template>
          <button
            type="button"
            class="mv-palette__row"
            data-testid="palette-submit"
            @click="submit"
          >
            <Search class="mv-palette__icon" />
            <span>{{
              raw.trim()
                ? t("shell.palette.searchFor", { query: raw.trim() })
                : t("shell.palette.openImages")
            }}</span>
            <kbd>Enter</kbd>
          </button>
          <template v-if="pages.length > 0">
            <div class="mv-palette__heading">{{ t("shell.palette.pages") }}</div>
            <button
              v-for="menu in pages"
              :key="menu.path"
              type="button"
              class="mv-palette__row"
              data-testid="palette-page"
              @click="open(menu)"
            >
              <component :is="menuIcons[menu.icon]" class="mv-palette__icon" />
              <span>{{ t(menu.name) }}</span>
            </button>
          </template>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.mv-palette-overlay {
  position: fixed;
  inset: 0;
  z-index: 3000;
  display: grid;
  place-items: start center;
  padding-top: 12vh;
  background: hsl(var(--overlay));
  backdrop-filter: blur(3px);
}
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
.mv-palette__row code {
  padding: 1px 6px;
  font-size: 12.5px;
  background: hsl(var(--muted));
  border-radius: 5px;
}
</style>
