<script setup lang="ts">
import { nextTick, onBeforeUnmount, useTemplateRef, watch } from "vue";

/**
 * Marvis 外壳的弹窗底座：遮罩、Esc/点遮罩关闭、Tab 焦点循环、关闭后把焦点还给触发元素。
 * 打开时聚焦带 data-autofocus 的元素，没有则聚焦第一个可聚焦元素。
 */
const show = defineModel<boolean>("show", { default: false });
defineProps<{ label: string; align?: "center" | "top" }>();
const panel = useTemplateRef<HTMLElement>("panel");

const FOCUSABLE = "input, button, textarea, select, a[href], [tabindex]:not([tabindex='-1'])";
function focusable(): HTMLElement[] {
  return [...(panel.value?.querySelectorAll<HTMLElement>(FOCUSABLE) ?? [])].filter(
    (element) => !element.hasAttribute("disabled"),
  );
}

let returnTo: HTMLElement | null = null;
watch(
  show,
  async (open) => {
    if (open) {
      returnTo = document.activeElement instanceof HTMLElement ? document.activeElement : null;
      await nextTick();
      const target = panel.value?.querySelector<HTMLElement>("[data-autofocus]") ?? focusable()[0];
      (target ?? panel.value)?.focus();
      return;
    }
    const target = returnTo;
    returnTo = null;
    await nextTick();
    if (target?.isConnected) target.focus();
  },
  { immediate: true },
);

function trapTab(event: KeyboardEvent): void {
  const items = focusable();
  if (items.length === 0) return;
  const first = items[0]!;
  const last = items[items.length - 1]!;
  // 焦点跑到弹窗外（或停在面板本身）时拉回来。
  if (!items.includes(document.activeElement as HTMLElement)) {
    event.preventDefault();
    (event.shiftKey ? last : first).focus();
    return;
  }
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault();
    last.focus();
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first.focus();
  }
}

// 监听挂在 document 上：焦点不在弹窗内时 Esc 与 Tab 也要生效。
function onKeydown(event: KeyboardEvent): void {
  if (event.isComposing) return;
  if (event.key === "Escape") {
    event.preventDefault();
    show.value = false;
  } else if (event.key === "Tab") trapTab(event);
}
watch(
  show,
  (open) => {
    if (open) document.addEventListener("keydown", onKeydown);
    else document.removeEventListener("keydown", onKeydown);
  },
  { immediate: true },
);
onBeforeUnmount(() => document.removeEventListener("keydown", onKeydown));
</script>

<template>
  <Teleport to="body">
    <div
      v-if="show"
      class="mv-dialog-overlay"
      :class="{ 'is-top': align === 'top' }"
      @mousedown.self="show = false"
    >
      <div
        ref="panel"
        class="mv-dialog"
        role="dialog"
        aria-modal="true"
        tabindex="-1"
        :aria-label="label"
      >
        <slot />
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.mv-dialog-overlay {
  position: fixed;
  inset: 0;
  z-index: 1900;
  display: grid;
  place-items: center;
  background: hsl(var(--overlay));
  backdrop-filter: blur(3px);
}
.mv-dialog-overlay.is-top {
  place-items: start center;
  padding-top: 12vh;
}
.mv-dialog {
  max-width: 100vw;
  max-height: 100%;
  outline: none;
}
</style>
