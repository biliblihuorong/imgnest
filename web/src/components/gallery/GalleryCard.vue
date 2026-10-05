<script setup lang="ts">
import { computed } from "vue";
import { NImage } from "naive-ui";
import type { GalleryItem } from "@/api/gallery";

/**
 * 画廊瀑布流卡片：缩略图优先（缺失回退原图直链），点击放大看原图。
 * 只展示名称与上传者用户名，不展示 EXIF/GPS/尺寸等私有信息。
 */
const props = defineProps<{
  item: GalleryItem;
}>();

/** 云缩略图缺失（空串）时回退 links.url。 */
const thumbSrc = computed(() => props.item.links.thumbnail_url || props.item.links.url);
/** 预览（点击放大）永远用原图直链。 */
const previewSrc = computed(() => props.item.links.url);
</script>

<template>
  <figure class="gallery-card">
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
      <span v-if="item.uploader" class="gallery-card__uploader" :title="`上传者：${item.uploader}`">
        {{ item.uploader }}
      </span>
    </figcaption>
  </figure>
</template>

<style scoped>
.gallery-card {
  break-inside: avoid;
  margin: 0 0 14px;
  border: 1px solid rgba(128, 128, 128, 0.25);
  border-radius: 8px;
  overflow: hidden;
  background: transparent;
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
  padding: 6px 10px 8px;
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
  color: rgba(128, 128, 128, 0.9);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 40%;
}
</style>
