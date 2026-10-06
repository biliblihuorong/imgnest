<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NButton, useMessage } from "naive-ui";
import { computed, onBeforeUnmount, ref } from "vue";
import type { ImageView } from "@/api/upload";
import { copyText } from "@/lib/clipboard";
import { buildLinkText, resolveImageLink, type LinkFormat, type LinkVersion } from "./linkText";

/**
 * 上传成功行的复制条：展示当前全局格式/版本下将被复制的文本，
 * 「复制」一键复制；链接文本可点击打开完整版本×格式复制抽屉。
 * 格式与版本由队列头部的全局控件统一持有。
 */
const props = defineProps<{ image: ImageView }>();
const format = defineModel<LinkFormat>("format", { required: true });
const version = defineModel<LinkVersion>("version", { required: true });
const emit = defineEmits<{ detail: [] }>();
const { t } = useI18n();
const message = useMessage();

const copying = ref(false);
let active = true;
onBeforeUnmount(() => {
  active = false;
});

const currentName = computed(() => props.image.name || props.image.key);
const currentLink = computed(() => resolveImageLink(props.image, version.value));
const copiedText = computed(() =>
  buildLinkText(format.value, currentName.value, currentLink.value),
);

async function copy(): Promise<void> {
  if (copying.value) return;
  copying.value = true;
  const ok = await copyText(copiedText.value);
  if (active) {
    if (ok) message.success(() => t("user.upload.copied"));
    else message.error(t("user.upload.copyError"));
    copying.value = false;
  }
}
</script>

<template>
  <div class="upload-result">
    <div
      class="upload-result__main"
      role="button"
      tabindex="0"
      @click="emit('detail')"
      @keydown.enter.prevent="emit('detail')"
    >
      <span class="upload-result__name" :title="currentName">{{ currentName }}</span>
      <span class="upload-result__link" :title="copiedText">{{ copiedText }}</span>
    </div>
    <div class="upload-result__actions">
      <NButton size="tiny" quaternary type="primary" @click="emit('detail')">
        {{ t("user.upload.allVersions") }}
      </NButton>
      <NButton
        size="small"
        type="primary"
        secondary
        :loading="copying"
        :disabled="copying"
        @click="copy"
        >{{ t("user.upload.copy") }}</NButton
      >
    </div>
  </div>
</template>

<style scoped>
.upload-result {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 12px;
  border-radius: 6px;
  background-color: hsl(var(--muted) / 0.5);
}

.upload-result__main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  cursor: pointer;
}

.upload-result__name {
  font-size: 13px;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.upload-result__link {
  font-size: 12px;
  color: hsl(var(--muted-foreground));
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.upload-result__actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}
</style>
