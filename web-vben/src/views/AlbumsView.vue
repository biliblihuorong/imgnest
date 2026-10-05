<script setup lang="ts">
import { Page } from "@vben/common-ui";
import { useI18n } from "@vben/locales";
import { NAlert, NButton, NEmpty, NPagination, NSpin } from "naive-ui";
import { shallowRef } from "vue";
import { useRouter } from "vue-router";
import type { AlbumView } from "@/api/albums";
import AlbumCard from "@/components/albums/AlbumCard.vue";
import AlbumFormModal from "@/components/albums/AlbumFormModal.vue";
import { ALBUM_PAGE_SIZE, useAlbums } from "@/components/albums/useAlbums";
import { formatApiError } from "@/locales/errors";

const { t } = useI18n();
const router = useRouter();
const { albums, total, page, loading, loadError, busyIds, load, changePage, remove } = useAlbums();
const modalShow = shallowRef(false);
const editing = shallowRef<AlbumView | null>(null);

function openForm(album: AlbumView | null): void {
  editing.value = album;
  modalShow.value = true;
}
</script>

<template>
  <Page :title="t('albums.title')" :description="t('albums.description')">
    <section class="albums-view">
      <div class="albums-view__toolbar rounded-lg border border-border bg-card p-4">
        <span class="text-sm text-muted-foreground">{{ t("albums.total", { count: total }) }}</span>
        <NButton type="primary" @click="openForm(null)">{{ t("albums.create") }}</NButton>
      </div>
      <NAlert v-if="loadError" type="error" :title="formatApiError(loadError, 'albums.loadFailed')">
        <NButton size="small" :loading="loading" @click="load">{{ t("albums.retry") }}</NButton>
      </NAlert>
      <NSpin :show="loading">
        <NEmpty
          v-if="!loading && !loadError && albums.length === 0"
          class="albums-view__empty rounded-lg border border-border bg-card"
          :description="t('albums.empty')"
        >
          <template #extra>
            <NButton size="small" @click="router?.push('/upload')">{{
              t("albums.upload")
            }}</NButton>
          </template>
        </NEmpty>
        <div v-else class="albums-view__grid">
          <AlbumCard
            v-for="album in albums"
            :key="album.id"
            :album="album"
            :busy="busyIds.has(album.id)"
            @edit="openForm(album)"
            @remove="remove(album)"
          />
        </div>
      </NSpin>
      <div class="albums-view__pagination">
        <NPagination
          :page="page"
          :item-count="total"
          :page-size="ALBUM_PAGE_SIZE"
          @update:page="changePage"
        />
      </div>
      <AlbumFormModal v-model:show="modalShow" :album="editing" @saved="load" />
    </section>
  </Page>
</template>

<style scoped>
.albums-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.albums-view__toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.albums-view__empty {
  padding: 64px 16px;
}
.albums-view__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 250px), 1fr));
  gap: 16px;
}
.albums-view__pagination {
  display: flex;
  justify-content: flex-end;
  overflow-x: auto;
}
</style>
