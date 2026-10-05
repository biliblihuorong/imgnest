<script setup lang="ts">
import { onMounted, ref } from "vue";
import { NAlert, NButton, NEmpty, NPagination, NSpin } from "naive-ui";
import { listGallery, type GalleryItem } from "@/api/gallery";
import { useSiteStore } from "@/stores/site";
import GalleryCard from "@/components/gallery/GalleryCard.vue";

/** 画廊固定每页 30 张。 */
const PAGE_SIZE = 30;

const site = useSiteStore();
void site.ensureLoaded();

const items = ref<GalleryItem[]>([]);
const total = ref(0);
const page = ref(1);
const loading = ref(false);
const loadError = ref(false);

onMounted(() => {
  void load();
});

/** 公开接口无需鉴权；画廊开关关闭时后端返回 200 空页，据此区分空态文案。 */
async function load(): Promise<void> {
  loading.value = true;
  loadError.value = false;
  try {
    const data = await listGallery({ page: page.value, size: PAGE_SIZE });
    items.value = data.items;
    total.value = data.total;
    page.value = data.page;
  } catch {
    loadError.value = true;
  } finally {
    loading.value = false;
  }
}

function handlePageChange(next: number): void {
  page.value = next;
  void load();
}

function retry(): void {
  void load();
}
</script>

<template>
  <section class="gallery-view">
    <header class="gallery-header">
      <h2 class="gallery-title">公开画廊</h2>
      <span v-if="!loadError" class="gallery-total">共 {{ total }} 张公开图片</span>
    </header>

    <NAlert v-if="loadError" type="error" title="画廊加载失败" class="gallery-alert">
      <div class="gallery-alert__body">
        <span>网络异常或服务暂时不可用，请稍后重试。</span>
        <NButton size="small" type="primary" :loading="loading" @click="retry">重试</NButton>
      </div>
    </NAlert>

    <NSpin v-else :show="loading">
      <NEmpty
        v-if="!loading && items.length === 0"
        class="gallery-empty"
        :description="site.galleryEnabled ? '暂无公开图片，公开的图片会展示在这里' : '站点未开放公开画廊'"
      />
      <div v-else class="gallery-masonry">
        <GalleryCard v-for="item in items" :key="item.id" :item="item" />
      </div>
    </NSpin>

    <div v-if="!loadError && total > 0" class="gallery-pagination">
      <NPagination
        :page="page"
        :item-count="total"
        :page-size="PAGE_SIZE"
        @update:page="handlePageChange"
      />
    </div>
  </section>
</template>

<style scoped>
.gallery-header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.gallery-title {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
}

.gallery-total {
  font-size: 13px;
  color: rgba(128, 128, 128, 0.9);
}

.gallery-alert__body {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.gallery-empty {
  padding: 48px 0;
}

.gallery-masonry {
  columns: 4;
  column-gap: 14px;
}

@media (max-width: 1100px) {
  .gallery-masonry {
    columns: 3;
  }
}

@media (max-width: 800px) {
  .gallery-masonry {
    columns: 2;
  }
}

.gallery-pagination {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
