<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NButton, NImage, NPopconfirm, NTag } from "naive-ui";
import type { AlbumView } from "@/api/albums";
import { formatDateTime } from "@/components/images/time";
import { useProtectedThumbnail } from "@/components/images/useProtectedThumbnail";

const props = defineProps<{ album: AlbumView; busy?: boolean }>();
const emit = defineEmits<{ edit: []; remove: [] }>();
const { t } = useI18n();
const { thumbnailUrl, failed } = useProtectedThumbnail(() => props.album.cover_thumb_url);
</script>

<template>
  <article class="album-card rounded-lg border border-border bg-card">
    <div class="album-card__cover bg-muted">
      <NImage
        v-if="thumbnailUrl && !failed"
        :src="thumbnailUrl"
        preview-disabled
        object-fit="cover"
        :img-props="{
          alt: album.name,
          onError: () => {
            failed = true;
          },
        }"
      />
      <div v-else class="album-card__placeholder text-muted-foreground">
        <svg
          width="32"
          height="32"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.5"
          aria-hidden="true"
        >
          <rect x="3" y="3" width="18" height="18" rx="3" />
          <circle cx="8" cy="8" r="1.5" />
          <path d="m3 17 5-5 4 4 4-6 5 7" />
        </svg>
        <span>{{ t("albums.noCover") }}</span>
      </div>
    </div>
    <div class="album-card__body">
      <div class="album-card__name" :title="album.name">{{ album.name }}</div>
      <div class="album-card__meta text-muted-foreground">
        <NTag size="small" :bordered="false" :type="album.is_public ? 'success' : 'default'">
          {{ t(album.is_public ? "albums.public" : "albums.private") }}
        </NTag>
        <span>{{ t("albums.imageCount", { count: album.image_count }) }}</span>
      </div>
      <div v-if="album.intro" class="album-card__intro text-muted-foreground" :title="album.intro">
        {{ album.intro }}
      </div>
      <time class="album-card__date text-muted-foreground" :datetime="album.created_at">{{
        formatDateTime(album.created_at)
      }}</time>
      <div class="album-card__actions border-t border-border">
        <NButton size="small" quaternary type="primary" :disabled="busy" @click="emit('edit')">{{
          t("albums.edit")
        }}</NButton>
        <NPopconfirm
          :positive-text="t('albums.confirmDelete')"
          :negative-text="t('albums.cancel')"
          :disabled="busy"
          @positive-click="!busy && emit('remove')"
        >
          <template #trigger>
            <NButton size="small" quaternary type="error" :disabled="busy" :loading="busy">{{
              t("albums.delete")
            }}</NButton>
          </template>
          {{ t("albums.deleteWarning") }}
        </NPopconfirm>
      </div>
    </div>
  </article>
</template>

<style scoped>
.album-card {
  overflow: hidden;
  min-width: 0;
}
.album-card__cover {
  height: 160px;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}
.album-card__cover :deep(.n-image),
.album-card__cover :deep(.n-image img) {
  width: 100%;
  height: 100%;
}
.album-card__placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  font-size: 12px;
}
.album-card__body {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.album-card__name {
  font-size: 15px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.album-card__meta {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 12px;
  flex-wrap: wrap;
}
.album-card__intro {
  font-size: 13px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.album-card__date {
  font-size: 12px;
}
.album-card__actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding-top: 10px;
}
</style>
