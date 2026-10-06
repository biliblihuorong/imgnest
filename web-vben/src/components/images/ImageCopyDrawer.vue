<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NButton, NDrawer, NDrawerContent, NTag, useMessage } from "naive-ui";
import { computed } from "vue";
import type { ImageView } from "@/api/images";
import { copyText } from "@/lib/clipboard";
import { buildLinkText, resolveImageLink, type LinkFormat, type LinkVersion } from "@/components/upload/linkText";

/**
 * 复制链接抽屉：一个图片的全部「版本 × 格式」矩阵。
 * 原图 / WebP / 缩略图 三段，每段展示链接文本和
 * 复制 URL / Markdown / HTML / BBCode 四个二级按钮。
 */
const props = defineProps<{ show: boolean; image: ImageView | null }>();
const emit = defineEmits<{ "update:show": [value: boolean] }>();
const { t } = useI18n();
const message = useMessage();

const formats: { value: LinkFormat; label: string }[] = [
  { value: "url", label: "URL" },
  { value: "markdown", label: "Markdown" },
  { value: "html", label: "HTML" },
  { value: "bbcode", label: "BBCode" },
];

interface VersionSection {
  value: LinkVersion;
  label: string;
  link: string;
  available: boolean;
}

const sections = computed<VersionSection[]>(() => {
  const image = props.image;
  if (!image) return [];
  return [
    {
      value: "original",
      label: t("user.upload.original"),
      link: image.links.original,
      available: image.links.original !== "",
    },
    {
      value: "webp",
      label: t("user.upload.webp"),
      link: image.links.webp,
      available: image.links.webp !== "",
    },
    {
      value: "thumbnail",
      label: t("user.upload.thumbnail"),
      link: image.links.thumbnail_url,
      available: image.links.thumbnail_url !== "",
    },
  ];
});

const name = computed(() => props.image?.name || props.image?.key || "");

async function copy(section: VersionSection, format: LinkFormat): Promise<void> {
  if (!props.image || !section.available) return;
  const text = buildLinkText(format, name.value, resolveImageLink(props.image, section.value));
  const formatLabel = formats.find((item) => item.value === format)?.label ?? format;
  const ok = await copyText(text);
  if (ok) {
    message.success(() =>
      t("user.upload.copiedCombo", { version: section.label, format: formatLabel }),
    );
  } else {
    message.error(() => t("user.upload.copyError"));
  }
}
</script>

<template>
  <NDrawer
    :show="props.show"
    width="min(460px, 100vw)"
    @update:show="emit('update:show', $event)"
  >
    <NDrawerContent :title="t('user.upload.linksTitle')" closable>
      <p class="copy-drawer__name" :title="name">{{ name }}</p>
      <section v-for="section in sections" :key="section.value" class="copy-drawer__section">
        <div class="copy-drawer__head">
          <span class="copy-drawer__version">{{ section.label }}</span>
          <NTag v-if="!section.available" size="small" :bordered="false">
            {{ t("user.upload.versionMissing") }}
          </NTag>
        </div>
        <p class="copy-drawer__link" :title="section.available ? section.link : ''">
          {{ section.available ? section.link : "—" }}
        </p>
        <div class="copy-drawer__actions">
          <NButton
            v-for="format in formats"
            :key="format.value"
            size="tiny"
            secondary
            type="primary"
            :disabled="!section.available"
            @click="copy(section, format.value)"
          >
            {{ t("user.upload.copyFormat", { format: format.label }) }}
          </NButton>
        </div>
      </section>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.copy-drawer__name {
  margin: 0 0 12px;
  font-size: 13px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.copy-drawer__section {
  padding: 12px;
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
  margin-bottom: 12px;
}

.copy-drawer__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}

.copy-drawer__version {
  font-size: 13px;
  font-weight: 600;
}

.copy-drawer__link {
  margin: 0 0 8px;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  overflow-wrap: anywhere;
}

.copy-drawer__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
</style>
