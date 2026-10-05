<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { useEventListener } from "@vueuse/core";
import { ref } from "vue";

const { t } = useI18n();

const props = defineProps<{ disabled?: boolean }>();
const emit = defineEmits<{ files: [files: File[]] }>();

const inputRef = ref<HTMLInputElement | null>(null);
const dragging = ref(false);

function pickFiles(): void {
  if (!props.disabled) {
    inputRef.value?.click();
  }
}

function onInputChange(event: Event): void {
  const input = event.target as HTMLInputElement;
  const files = Array.from(input.files ?? []);
  // 清空 value，允许再次选择同一批文件
  input.value = "";
  if (props.disabled || files.length === 0) {
    return;
  }
  emit("files", files);
}

function onDragOver(): void {
  if (!props.disabled) {
    dragging.value = true;
  }
}

function onDrop(event: DragEvent): void {
  dragging.value = false;
  if (props.disabled) {
    return;
  }
  const files = Array.from(event.dataTransfer?.files ?? []);
  if (files.length > 0) {
    emit("files", files);
  }
}

function onPaste(event: ClipboardEvent): void {
  if (props.disabled) {
    return;
  }
  const files = Array.from(event.clipboardData?.files ?? []);
  if (files.length > 0) {
    emit("files", files);
  }
}

// 粘贴在整个页面生效；同时阻止浏览器用默认行为打开拖入的文件
useEventListener(document, "paste", onPaste);
useEventListener(window, "dragover", (event) => event.preventDefault());
useEventListener(window, "drop", (event) => event.preventDefault());
</script>

<template>
  <div
    class="upload-dropzone"
    :class="{ 'is-dragging': dragging, 'is-disabled': disabled }"
    role="button"
    :aria-disabled="disabled ?? false"
    :tabindex="disabled ? -1 : 0"
    @click="pickFiles"
    @keydown.enter.prevent="pickFiles"
    @keydown.space.prevent="pickFiles"
    @dragover.prevent="onDragOver"
    @dragleave="dragging = false"
    @drop.prevent="onDrop"
  >
    <p class="upload-dropzone__title">{{ t("user.upload.dropTitle") }}</p>
    <p class="upload-dropzone__hint">{{ t("user.upload.dropHint") }}</p>
    <input
      ref="inputRef"
      class="upload-dropzone__input"
      type="file"
      accept="image/*"
      multiple
      @change="onInputChange"
      @click.stop
    />
  </div>
</template>

<style scoped>
.upload-dropzone {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  min-height: 220px;
  padding: 40px 24px;
  border: 1px dashed hsl(var(--border));
  background: hsl(var(--muted) / 0.35);
  border-radius: 6px;
  text-align: center;
  cursor: pointer;
  user-select: none;
  transition:
    border-color 0.2s,
    background-color 0.2s;
}

.upload-dropzone:hover,
.upload-dropzone:focus-visible,
.upload-dropzone.is-dragging {
  border-color: hsl(var(--primary));
}

.upload-dropzone.is-dragging {
  background-color: hsl(var(--primary) / 0.06);
}

.upload-dropzone.is-disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.upload-dropzone__title {
  margin: 0;
  font-size: 15px;
}

.upload-dropzone__hint {
  margin: 0;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
  line-height: 1.7;
}

.upload-dropzone__input {
  display: none;
}
@media (max-width: 640px) {
  .upload-dropzone {
    min-height: 190px;
    padding: 32px 16px;
  }
}
</style>
