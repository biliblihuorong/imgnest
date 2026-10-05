<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NAlert, NButton, NEmpty, NSpin } from "naive-ui";
import { onMounted, onScopeDispose, shallowRef } from "vue";
import { listImages, type ImageView } from "@/api/images";
import { formatApiError } from "@/locales/errors";
import AlbumCoverOption from "./AlbumCoverOption.vue";

withDefaults(defineProps<{ selected: number; disabled?: boolean }>(), { disabled: false });
const emit = defineEmits<{ select: [id: number] }>();
const { t } = useI18n();
const images = shallowRef<ImageView[]>([]);
const loading = shallowRef(false);
const error = shallowRef<unknown>(null);
let active = true;

async function load(): Promise<void> {
  if (loading.value || !active) return;
  loading.value = true;
  error.value = null;
  try {
    const data = await listImages({ page: 1, size: 12 });
    if (active) images.value = data.items;
  } catch (cause) {
    if (active) error.value = cause;
  } finally {
    if (active) loading.value = false;
  }
}
onMounted(() => void load());
onScopeDispose(() => {
  active = false;
});
</script>

<template>
  <div class="album-form__picker">
    <NSpin :show="loading" size="small">
      <NAlert v-if="error" type="error" :title="formatApiError(error, 'albums.pickerFailed')">
        <NButton size="small" :disabled="disabled" @click="load">{{ t("albums.retry") }}</NButton>
      </NAlert>
      <div v-else-if="images.length > 0" class="album-form__picker-grid">
        <AlbumCoverOption
          v-for="image in images"
          :key="image.id"
          :image="image"
          :selected="selected === image.id"
          :disabled="disabled"
          @select="emit('select', $event)"
        />
      </div>
      <NEmpty v-else-if="!loading" :description="t('albums.pickerEmpty')" size="small" />
    </NSpin>
  </div>
</template>

<style scoped>
.album-form__picker {
  margin: 4px 0 16px;
  min-height: 64px;
}
.album-form__picker-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(84px, 1fr));
  gap: 8px;
}
</style>
