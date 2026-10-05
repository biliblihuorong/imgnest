<script setup lang="ts">
import { useI18n } from "@vben/locales";
import {
  NAlert,
  NButton,
  NEmpty,
  NInput,
  NModal,
  NPagination,
  NSpin,
  NTag,
} from "naive-ui";
import { onMounted, onScopeDispose, ref, shallowRef } from "vue";
import { watchDebounced } from "@vueuse/core";
import { listImages, type ImageView } from "@/api/images";
import { formatApiError } from "@/locales/errors";
import AlbumCoverOption from "./AlbumCoverOption.vue";

/**
 * 全屏封面挑选器：从「我的图片」中挑一张作为相册封面。
 * 大图网格 + 关键词搜索 + 分页，选中后回填表单并关闭。
 */
withDefaults(defineProps<{ selected: number; disabled?: boolean }>(), { disabled: false });
const emit = defineEmits<{ select: [id: number] }>();
const { t } = useI18n();
const images = shallowRef<ImageView[]>([]);
const loading = shallowRef(false);
const error = shallowRef<unknown>(null);
const search = ref("");
const page = ref(1);
const size = 24;
const total = shallowRef(0);
let requestId = 0;
let active = true;

async function load(): Promise<void> {
  if (loading.value || !active) return;
  const current = ++requestId;
  loading.value = true;
  error.value = null;
  try {
    const data = await listImages({
      page: page.value,
      size,
      q: search.value.trim() || undefined,
    });
    if (!active || current !== requestId) return;
    images.value = data.items;
    total.value = data.total;
  } catch (cause) {
    if (active && current === requestId) error.value = cause;
  } finally {
    if (active && current === requestId) loading.value = false;
  }
}

function handlePageChange(next: number): void {
  page.value = next;
  void load();
}

watchDebounced(
  search,
  () => {
    page.value = 1;
    void load();
  },
  { debounce: 350, maxWait: 1000 },
);

onMounted(() => void load());
onScopeDispose(() => {
  active = false;
  requestId++;
});
</script>

<template>
  <NModal
    :show="true"
    preset="card"
    :title="t('albums.pickerTitle')"
    class="album-cover-picker"
    :style="{ width: '100vw', height: '100vh', maxWidth: '100vw' }"
    :bordered="false"
    content-style="display: flex; flex-direction: column; min-height: 0;"
  >
    <div class="album-cover-picker__toolbar">
      <NInput
        v-model:value="search"
        class="album-cover-picker__search"
        size="small"
        clearable
        :placeholder="t('albums.pickerSearchPlaceholder')"
        :aria-label="t('albums.pickerSearch')"
      />
      <NButton size="small" :disabled="loading" @click="load">{{ t("albums.retry") }}</NButton>
      <NTag v-if="selected > 0" size="small" :bordered="false" type="primary">
        {{ t("albums.coverImage", { id: selected }) }}
      </NTag>
    </div>
    <div class="album-cover-picker__body">
      <NSpin :show="loading" size="small" class="album-cover-picker__spin" content-style="min-height: 0; flex: 1;">
        <NAlert v-if="error" type="error" :title="formatApiError(error, 'albums.pickerFailed')">
          <NButton size="small" :disabled="loading" @click="load">{{ t("albums.retry") }}</NButton>
        </NAlert>
        <div v-else-if="images.length > 0" class="album-cover-picker__grid">
          <AlbumCoverOption
            v-for="image in images"
            :key="image.id"
            :image="image"
            :selected="selected === image.id"
            :disabled="disabled"
            @select="emit('select', $event)"
          />
        </div>
        <NEmpty
          v-else-if="!loading"
          class="album-cover-picker__empty"
          :description="t('albums.pickerEmpty')"
        />
      </NSpin>
    </div>
    <div class="album-cover-picker__pagination">
      <NPagination
        :page="page"
        :item-count="total"
        :page-size="size"
        @update:page="handlePageChange"
      />
    </div>
  </NModal>
</template>

<style scoped>
.album-cover-picker__toolbar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 12px;
}

.album-cover-picker__search {
  width: 260px;
}

.album-cover-picker__body {
  flex: 1;
  min-height: 0;
  overflow: auto;
}

.album-cover-picker__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 12px;
}

.album-cover-picker__grid :deep(.album-form__picker-item) {
  height: 130px;
}

.album-cover-picker__empty {
  padding: 64px 0;
}

.album-cover-picker__pagination {
  display: flex;
  justify-content: flex-end;
  padding-top: 12px;
}
</style>
