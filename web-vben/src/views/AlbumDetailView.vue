<script setup lang="ts">
import { Page } from "@vben/common-ui";
import { useI18n } from "@vben/locales";
import { NButton } from "naive-ui";
import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";
import ImageLibrary from "@/components/images/ImageLibrary.vue";

/**
 * 相册详情：锁定到单个相册的图片库，与「我的图片」共用同一套
 * 筛选/搜索/灯箱/右键菜单/批量操作组件。
 */
const route = useRoute();
const router = useRouter();
const { t } = useI18n();

const albumId = computed(() => Number(route.params.id));
const albumName = computed(() => {
  const name = route.query.name;
  return typeof name === "string" && name.trim() !== "" ? name : t("albums.detailFallback");
});

function backToAlbums(): void {
  void router.push({ name: "albums" });
}
</script>

<template>
  <Page class="album-detail-view" header-class="flex-wrap gap-4">
    <template #title>
      <div class="album-detail-view__heading">
        <NButton size="small" quaternary @click="backToAlbums">
          ← {{ t("albums.backToList") }}
        </NButton>
        <h1 class="album-detail-view__title" :title="albumName">{{ albumName }}</h1>
      </div>
    </template>
    <ImageLibrary v-if="albumId > 0" :locked-album-id="albumId" />
  </Page>
</template>

<style scoped>
.album-detail-view__heading {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.album-detail-view__title {
  margin: 0 0 8px;
  font-size: 18px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
