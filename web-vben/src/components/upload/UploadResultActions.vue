<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NButton, NRadio, NRadioGroup, useMessage } from "naive-ui";
import { computed, onBeforeUnmount, ref, useId } from "vue";
import type { ImageView } from "@/api/upload";
import { buildLinkText, type LinkFormat } from "./linkText";

type LinkVersion = "original" | "webp" | "thumbnail";

interface VersionOption {
  value: LinkVersion;
  label: string;
  link: string;
  available: boolean;
}

const { t } = useI18n();

const props = defineProps<{ image: ImageView }>();
const message = useMessage();

const groupId = useId();
const copying = ref(false);
let active = true;
onBeforeUnmount(() => {
  active = false;
});

const format = ref<LinkFormat>("url");
const version = ref<LinkVersion>(initialVersion(props.image));

function initialVersion(image: ImageView): LinkVersion {
  // 默认选第一个可用的版本：原图 → WebP → 缩略图
  if (image.links.original !== "") {
    return "original";
  }
  if (image.links.webp !== "") {
    return "webp";
  }
  return "thumbnail";
}

const versionOptions = computed<VersionOption[]>(() => [
  {
    value: "original",
    label: t("user.upload.original"),
    link: props.image.links.original,
    available: props.image.links.original !== "",
  },
  {
    value: "webp",
    label: t("user.upload.webp"),
    link: props.image.links.webp,
    available: props.image.links.webp !== "",
  },
  {
    value: "thumbnail",
    label: t("user.upload.thumbnail"),
    link: props.image.links.thumbnail_url,
    available: props.image.links.thumbnail_url !== "",
  },
]);

const currentLink = computed(() => {
  const option = versionOptions.value.find((item) => item.value === version.value);
  // 选中版本缺失时回退到首选可用链接
  return option?.link || props.image.links.url;
});

const currentName = computed(() => props.image.name || props.image.key);

const copiedText = computed(() =>
  buildLinkText(format.value, currentName.value, currentLink.value),
);

async function copy(): Promise<void> {
  if (copying.value) return;
  copying.value = true;
  try {
    await navigator.clipboard.writeText(copiedText.value);
    if (active) message.success(() => t("user.upload.copied"));
  } catch {
    if (active) message.error(t("user.upload.copyError"));
  } finally {
    if (active) copying.value = false;
  }
}
</script>

<template>
  <div class="upload-result">
    <NRadioGroup v-model:value="format" :name="`${groupId}-format`" size="small">
      <NRadio value="url">URL</NRadio>
      <NRadio value="markdown">Markdown</NRadio>
      <NRadio value="html">HTML</NRadio>
      <NRadio value="bbcode">BBCode</NRadio>
    </NRadioGroup>
    <NRadioGroup v-model:value="version" :name="`${groupId}-version`" size="small">
      <NRadio
        v-for="option in versionOptions"
        :key="option.value"
        :value="option.value"
        :disabled="!option.available"
      >
        {{ option.label }}
      </NRadio>
    </NRadioGroup>
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
</template>

<style scoped>
.upload-result {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 16px;
  padding: 8px 12px;
  border-radius: 6px;
  background-color: hsl(var(--muted) / 0.5);
}
.upload-result :deep(.n-radio-group) {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 0;
}
</style>
