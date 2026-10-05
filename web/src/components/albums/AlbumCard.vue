<script setup lang="ts">
import { computed, ref } from "vue";
import { NButton, NImage, NPopconfirm, NTag } from "naive-ui";
import type { AlbumView } from "@/api/albums";
import { formatDateTime } from "@/components/images/time";

const props = defineProps<{
  album: AlbumView;
  /** 该卡片有待完成的写操作（删除）时禁用控件。 */
  busy?: boolean;
}>();

const emit = defineEmits<{
  edit: [];
  remove: [];
}>();

const coverFailed = ref(false);
const coverUrl = computed(() => (coverFailed.value ? "" : props.album.cover_thumb_url));

function onCoverError(): void {
  coverFailed.value = true;
}
</script>

<template>
  <div class="album-card">
    <div class="album-card__cover">
      <NImage
        v-if="coverUrl"
        :src="coverUrl"
        preview-disabled
        object-fit="cover"
        :img-props="{ alt: album.name, onError: onCoverError }"
      />
      <div v-else class="album-card__placeholder">
        <span class="album-card__placeholder-mark">IMG</span>
        <span class="album-card__placeholder-hint">无封面</span>
      </div>
    </div>
    <div class="album-card__body">
      <div class="album-card__name" :title="album.name">{{ album.name }}</div>
      <div class="album-card__meta">
        <NTag size="small" :bordered="false" :type="album.is_public ? 'success' : 'default'">
          {{ album.is_public ? "公开" : "私有" }}
        </NTag>
        <span>{{ album.image_count }} 张图片</span>
        <span>{{ formatDateTime(album.created_at) }}</span>
      </div>
      <div v-if="album.intro" class="album-card__intro" :title="album.intro">
        {{ album.intro }}
      </div>
      <div class="album-card__actions">
        <NButton size="tiny" quaternary type="primary" :disabled="busy" @click="emit('edit')">
          编辑
        </NButton>
        <div class="album-card__spacer" />
        <NPopconfirm
          positive-text="确认删除"
          negative-text="取消"
          @positive-click="emit('remove')"
        >
          <template #trigger>
            <NButton size="tiny" quaternary type="error" :disabled="busy">删除</NButton>
          </template>
          相册内图片会保留，仅移出相册。确定删除该相册吗？
        </NPopconfirm>
      </div>
    </div>
  </div>
</template>

<style scoped>
.album-card {
  border: 1px solid rgba(128, 128, 128, 0.25);
  border-radius: 8px;
  overflow: hidden;
  background: transparent;
}

.album-card__cover {
  height: 130px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(128, 128, 128, 0.08);
  overflow: hidden;
}

.album-card__cover :deep(.n-image) {
  width: 100%;
  height: 100%;
}

.album-card__cover :deep(.n-image img) {
  width: 100%;
  height: 100%;
}

.album-card__placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  color: rgba(128, 128, 128, 0.8);
}

.album-card__placeholder-mark {
  font-size: 22px;
  font-weight: 600;
  letter-spacing: 1px;
}

.album-card__placeholder-hint {
  font-size: 12px;
}

.album-card__body {
  padding: 8px 10px 10px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.album-card__name {
  font-size: 14px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.album-card__meta {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: rgba(128, 128, 128, 0.9);
  flex-wrap: wrap;
}

.album-card__intro {
  font-size: 12px;
  color: rgba(128, 128, 128, 0.9);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.album-card__actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.album-card__spacer {
  flex: 1;
}
</style>
