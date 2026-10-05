<script setup lang="ts">
import { Page } from "@vben/common-ui";
import { useI18n } from "@vben/locales";
import { NAlert, NButton, NEmpty, NPagination, NSpin } from "naive-ui";
import GalleryCard from "@/components/gallery/GalleryCard.vue";
import { GALLERY_PAGE_SIZE, useGallery } from "@/components/gallery/useGallery";

const { t } = useI18n();
const { site, items, total, page, loading, loadError, changePage, retry } = useGallery();
</script>

<template>
  <Page :title="t('gallery.title')" :description="t('gallery.description')">
    <section class="gallery-view">
      <div v-if="!loadError" class="gallery-total text-sm text-muted-foreground">
        {{ t("gallery.total", { count: total }, total) }}
      </div>
      <NAlert v-if="loadError" type="error" :title="t('gallery.loadFailed')">
        <div class="gallery-alert__body">
          <span>{{ t("gallery.errorDescription") }}</span>
          <NButton size="small" type="primary" :loading="loading" @click="retry">{{
            t("gallery.retry")
          }}</NButton>
        </div>
      </NAlert>
      <NSpin v-else :show="loading">
        <NEmpty
          v-if="!loading && (!site.galleryEnabled || items.length === 0)"
          class="gallery-empty rounded-lg border border-border bg-card"
          :description="t(site.galleryEnabled ? 'gallery.empty' : 'gallery.disabled')"
        />
        <div v-else-if="site.galleryEnabled" class="gallery-masonry">
          <GalleryCard v-for="item in items" :key="item.id" :item="item" />
        </div>
      </NSpin>
      <div v-if="!loadError && site.galleryEnabled && total > 0" class="gallery-pagination">
        <NPagination
          :page="page"
          :item-count="total"
          :page-size="GALLERY_PAGE_SIZE"
          @update:page="changePage"
        />
      </div>
    </section>
  </Page>
</template>

<style scoped>
.gallery-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.gallery-alert__body {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
.gallery-empty {
  padding: 64px 16px;
}
.gallery-masonry {
  columns: 4;
  column-gap: 16px;
}
.gallery-pagination {
  display: flex;
  justify-content: flex-end;
  overflow-x: auto;
}
@media (max-width: 1280px) {
  .gallery-masonry {
    columns: 3;
  }
}
@media (max-width: 900px) {
  .gallery-masonry {
    columns: 2;
  }
}
@media (max-width: 520px) {
  .gallery-masonry {
    columns: 1;
  }
}
</style>
