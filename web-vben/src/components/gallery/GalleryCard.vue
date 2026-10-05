<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "@vben/locales";
import { NImage } from "naive-ui";
import type { GalleryItem } from "@/api/gallery";

/**
 * 画廊瀑布流卡片：缩略图优先（缺失回退原图直链），点击放大看原图。
 * 只展示名称与上传者用户名，不展示 EXIF/GPS/尺寸等私有信息。
 */
const props = defineProps<{
  item: GalleryItem;
}>();

const { t } = useI18n();

/** 云缩略图缺失（空串）时回退 links.url。 */
const thumbSrc = computed(() => props.item.links.thumbnail_url || props.item.links.url);
/** 预览（点击放大）永远用原图直链。 */
const previewSrc = computed(() => props.item.links.url);
</script>

<template>
  <figure class="gallery-card rounded-lg border border-border bg-card">
    <NImage
      class="gallery-card__image"
      :src="thumbSrc"
      :preview-src="previewSrc"
      lazy
      object-fit="cover"
      :img-props="{ alt: item.name }"
    />
    <figcaption class="gallery-card__caption">
      <span class="gallery-card__name" :title="item.name">{{ item.name }}</span>
      <span
        v-if="item.uploader"
        class="gallery-card__uploader"
        :title="t('gallery.uploader', { name: item.uploader })"
      >
        {{ item.uploader }}
      </span>
    </figcaption>
  </figure>
</template>

<style scoped>
.gallery-card {
  break-inside: avoid;
  margin: 0 0 16px;
  overflow: hidden;
}

.gallery-card__image {
  display: block;
  width: 100%;
}

.gallery-card__image :deep(img) {
  display: block;
  width: 100%;
  height: auto;
}

.gallery-card__caption {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 12px;
}

.gallery-card__name {
  font-size: 13px;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.gallery-card__uploader {
  flex: none;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 40%;
}
</style>
