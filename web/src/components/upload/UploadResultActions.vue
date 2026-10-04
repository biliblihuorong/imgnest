<script setup lang="ts">
import { NButton, NRadio, NRadioGroup, useMessage } from "naive-ui";
import { computed, ref } from "vue";
import type { ImageView } from "@/api/upload";
import { buildLinkText, type LinkFormat } from "./linkText";

type LinkVersion = "original" | "webp" | "thumbnail";

interface VersionOption {
  value: LinkVersion;
  label: string;
  link: string;
  available: boolean;
}

const props = defineProps<{ image: ImageView }>();
const message = useMessage();

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
  { value: "original", label: "原图", link: props.image.links.original, available: props.image.links.original !== "" },
  { value: "webp", label: "WebP", link: props.image.links.webp, available: props.image.links.webp !== "" },
  { value: "thumbnail", label: "缩略图", link: props.image.links.thumbnail_url, available: props.image.links.thumbnail_url !== "" },
]);

const currentLink = computed(() => {
  const option = versionOptions.value.find((item) => item.value === version.value);
  // 选中版本缺失时回退到首选可用链接
  return option?.link || props.image.links.url;
});

const currentName = computed(() => props.image.name || props.image.key);

const copiedText = computed(() => buildLinkText(format.value, currentName.value, currentLink.value));

async function copy(): Promise<void> {
  try {
    await navigator.clipboard.writeText(copiedText.value);
    message.success("已复制到剪贴板");
  } catch {
    message.error("复制失败，请手动复制");
  }
}
</script>

<template>
  <div class="upload-result">
    <NRadioGroup v-model:value="format" name="link-format" size="small">
      <NRadio value="url">URL</NRadio>
      <NRadio value="markdown">Markdown</NRadio>
      <NRadio value="html">HTML</NRadio>
      <NRadio value="bbcode">BBCode</NRadio>
    </NRadioGroup>
    <NRadioGroup v-model:value="version" name="link-version" size="small">
      <NRadio
        v-for="option in versionOptions"
        :key="option.value"
        :value="option.value"
        :disabled="!option.available"
      >
        {{ option.label }}
      </NRadio>
    </NRadioGroup>
    <NButton size="small" type="primary" secondary @click="copy">复制</NButton>
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
  background-color: rgba(128, 128, 128, 0.08);
}
</style>
