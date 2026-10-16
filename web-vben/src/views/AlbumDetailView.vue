<script setup lang="ts">
import { Page } from "@vben/common-ui";
import { useI18n } from "@vben/locales";
import { Link2 } from "@vben/icons";
import { NButton, NModal } from "naive-ui";
import { computed, shallowRef } from "vue";
import { useRoute, useRouter } from "vue-router";
import AlbumRandomLink from "@/components/albums/AlbumRandomLink.vue";
import ImageLibrary from "@/components/images/ImageLibrary.vue";

/**
 * 相册详情：锁定到单个相册的图片库，与「我的图片」共用同一套
 * 筛选/搜索/灯箱/右键菜单/批量操作组件。随机图片链接收在标题栏按钮的弹窗里，不占图片区域。
 */
const route = useRoute();
const router = useRouter();
const { t } = useI18n();

const albumId = computed(() => {
  const value = route.params.id;
  return typeof value === "string" && /^[1-9][0-9]*$/.test(value) ? value : "";
});
const albumName = computed(() => {
  const name = route.query.name;
  return typeof name === "string" && name.trim() !== "" ? name : t("albums.detailFallback");
});

const randomLinkOpen = shallowRef(false);

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
    <template v-if="albumId" #extra>
      <NButton size="small" data-testid="album-random-link" @click="randomLinkOpen = true">
        <template #icon><Link2 /></template>
        {{ t("albums.randomLink.title") }}
      </NButton>
    </template>
    <NModal
      v-model:show="randomLinkOpen"
      preset="card"
      :style="{ width: 'min(560px, calc(100vw - 32px))' }"
      :title="t('albums.randomLink.title')"
      :bordered="false"
    >
      <AlbumRandomLink v-if="albumId" :album-id="albumId" embedded />
    </NModal>
    <ImageLibrary
      v-if="albumId"
      :key="albumId"
      :locked-album-id="albumId"
      :locked-album-name="albumName"
    />
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
