<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NAlert, NButton, NEmpty, NImageGroup, NSpin } from "naive-ui";
import { onScopeDispose, shallowRef, watch } from "vue";
import { previewGallery } from "@/api/admin";
import type { components } from "@/api/schema";
import GalleryCard from "@/components/gallery/GalleryCard.vue";
import { usePreviewSwipe } from "@/components/images/usePreviewSwipe";

const props = defineProps<{ publicAlbumsOnly: boolean }>();
const { t } = useI18n();
const page = shallowRef<components["schemas"]["GalleryPage"] | null>(null);
const loading = shallowRef(false);
const failed = shallowRef(false);
const expanded = shallowRef(false);
const renderToolbar = usePreviewSwipe();
let requestId = 0;

async function load(): Promise<void> {
  const current = ++requestId;
  loading.value = true;
  failed.value = false;
  page.value = null;
  try {
    const result = await previewGallery(props.publicAlbumsOnly);
    if (current === requestId) page.value = result;
  } catch {
    if (current === requestId) failed.value = true;
  } finally {
    if (current === requestId) loading.value = false;
  }
}

watch(
  () => props.publicAlbumsOnly,
  () => void load(),
  { immediate: true },
);
onScopeDispose(() => {
  requestId += 1;
});
</script>

<template>
  <section class="gallery-preview" :aria-label="t('admin.settings.galleryPreview')">
    <h3 class="gallery-preview__title">{{ t("admin.settings.galleryPreview") }}</h3>
    <p class="gallery-preview__hint">{{ t("admin.settings.galleryPreviewHint") }}</p>
    <NAlert v-if="failed" type="error" data-gallery-preview-error>
      {{ t("admin.settings.galleryPreviewFailed") }}
      <NButton size="small" secondary data-action="retry-preview" @click="load">{{
        t("admin.common.retry")
      }}</NButton>
    </NAlert>
    <NSpin v-else :show="loading">
      <template v-if="page">
        <p data-gallery-count>{{ t("admin.settings.galleryCount", { count: page.total }) }}</p>
        <NButton size="small" secondary data-action="toggle-preview" @click="expanded = !expanded">
          {{
            t(expanded ? "admin.settings.hideGalleryPreview" : "admin.settings.showGalleryPreview")
          }}
        </NButton>
        <template v-if="expanded">
          <NEmpty v-if="page.items.length === 0" :description="t('gallery.empty')" />
          <NImageGroup v-else :render-toolbar="renderToolbar">
            <div class="gallery-preview__grid">
              <GalleryCard v-for="item in page.items" :key="item.id" :item="item" />
            </div>
          </NImageGroup>
        </template>
      </template>
    </NSpin>
  </section>
</template>

<style scoped>
.gallery-preview {
  margin-top: 16px;
}
.gallery-preview__title {
  margin: 0 0 8px;
  font-size: 15px;
  font-weight: 600;
}
.gallery-preview__hint {
  margin: 0 0 12px;
  color: hsl(var(--muted-foreground));
}
.gallery-preview__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(180px, 100%), 1fr));
  gap: 12px;
  margin-top: 16px;
}
</style>
