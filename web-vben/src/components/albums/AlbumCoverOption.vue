<script setup lang="ts">
import { useI18n } from "@vben/locales";
import type { ImageView } from "@/api/images";
import { useProtectedThumbnail } from "@/components/images/useProtectedThumbnail";

const props = defineProps<{ image: ImageView; selected: boolean; disabled: boolean }>();
const emit = defineEmits<{ select: [id: number] }>();
const { t } = useI18n();
const { thumbnailUrl, failed } = useProtectedThumbnail(() => props.image.local_thumb_url);
</script>

<template>
  <button
    type="button"
    class="album-form__picker-item"
    :class="{ 'album-form__picker-item--active': selected }"
    :title="image.name"
    :aria-label="t('albums.selectCover', { name: image.name })"
    :aria-pressed="selected"
    :disabled="disabled"
    @click="emit('select', image.id)"
  >
    <img
      v-if="thumbnailUrl && !failed"
      :src="thumbnailUrl"
      :alt="image.name"
      @error="failed = true"
    />
    <span v-else class="album-form__picker-fallback text-muted-foreground">{{ image.name }}</span>
  </button>
</template>

<style scoped>
.album-form__picker-item {
  height: 76px;
  padding: 0;
  border: 2px solid transparent;
  border-radius: 6px;
  overflow: hidden;
  cursor: pointer;
  background: hsl(var(--muted));
  display: flex;
  align-items: center;
  justify-content: center;
}
.album-form__picker-item--active {
  border-color: hsl(var(--primary));
}
.album-form__picker-item:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}
.album-form__picker-item img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.album-form__picker-fallback {
  font-size: 12px;
  padding: 0 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
