<script setup lang="ts">
import { Page } from "@vben/common-ui";
import { useI18n } from "@vben/locales";
import {
  NAlert,
  NButton,
  NCard,
  NEmpty,
  NPagination,
  NPopconfirm,
  NSelect,
  NSpin,
  NTabPane,
  NTabs,
} from "naive-ui";
import TrashView from "./TrashView.vue";
import ImageCard from "@/components/images/ImageCard.vue";
import ImageDetailDrawer from "@/components/images/ImageDetailDrawer.vue";
import { formatApiError } from "@/locales/errors";
import { useImageLibrary } from "@/components/images/useImageLibrary";
const { t } = useI18n();
const {
  sizeOptions,
  images,
  total,
  page,
  size,
  loading,
  loadError,
  busyIds,
  drawerShow,
  drawerImage,
  albumFilter,
  selectedIds,
  batchLoading,
  batchTarget,
  albumFilterOptions,
  batchTargetOptions,
  cardMoveOptions,
  load,
  handlePageChange,
  handleSizeChange,
  handleAlbumFilterChange,
  handleToggle,
  handleRemove,
  handleSelect,
  clearSelection,
  moveSelected,
  removeSelected,
  batchVisibility,
  handleMove,
  openDrawer,
} = useImageLibrary();
</script>

<template>
  <Page
    class="images-view"
    :description="t('user.images.description')"
    header-class="flex-wrap gap-4"
  >
    <template #title
      ><h1 class="images-view__title">{{ t("user.images.title") }}</h1></template
    >
    <NCard :bordered="false">
      <NTabs default-value="images" type="line">
        <NTabPane name="images" :tab="t('user.images.tab')">
          <div class="images-toolbar">
            <span class="images-total">{{ t("user.images.total", { count: total }) }}</span>
            <div class="images-toolbar__filters">
              <NSelect
                class="images-size"
                size="small"
                :value="size"
                :options="sizeOptions"
                :aria-label="t('user.images.pageSize')"
                @update:value="handleSizeChange"
              />
              <NSelect
                class="images-album-filter"
                size="small"
                :value="albumFilter"
                :options="albumFilterOptions"
                :placeholder="t('user.images.albumFilter')"
                clearable
                @update:value="handleAlbumFilterChange"
              />
              <NButton
                size="small"
                :loading="loading"
                :disabled="loading || batchLoading"
                @click="load"
                >{{ t("user.common.refresh") }}</NButton
              >
            </div>
          </div>
          <div v-if="selectedIds.length > 0" class="images-batchbar">
            <span class="images-batchbar__count">{{
              t("user.images.selected", { count: selectedIds.length })
            }}</span>
            <NSelect
              v-model:value="batchTarget"
              class="images-batchbar__album"
              size="small"
              :options="batchTargetOptions"
              :placeholder="t('user.images.targetAlbum')"
              :disabled="batchLoading"
            />
            <NButton size="small" type="primary" :loading="batchLoading" @click="moveSelected">
              {{ t("user.images.moveAlbum") }}
            </NButton>
            <NPopconfirm
              :positive-text="t('user.common.confirmDelete')"
              :negative-text="t('user.common.cancel')"
              @positive-click="removeSelected"
            >
              <template #trigger>
                <NButton size="small" type="error" :loading="batchLoading">{{
                  t("user.images.batchDelete")
                }}</NButton>
              </template>
              {{ t("user.images.batchDeleteConfirm", { count: selectedIds.length }) }}
            </NPopconfirm>
            <NButton size="small" :disabled="batchLoading" @click="batchVisibility(true)">
              {{ t("user.images.makePublic") }}
            </NButton>
            <NButton size="small" :disabled="batchLoading" @click="batchVisibility(false)">
              {{ t("user.images.makePrivate") }}
            </NButton>
            <NButton size="small" quaternary :disabled="batchLoading" @click="clearSelection">
              {{ t("user.common.cancel") }}
            </NButton>
          </div>
          <NAlert v-if="loadError" type="error" class="images-error">
            {{ formatApiError(loadError, "user.images.listError") }}
            <NButton text :disabled="loading" @click="load">{{ t("user.common.retry") }}</NButton>
          </NAlert>
          <NSpin v-else :show="loading">
            <NEmpty
              v-if="!loading && images.length === 0"
              class="images-empty"
              :description="
                t(albumFilter === null ? 'user.images.empty' : 'user.images.albumEmpty')
              "
            />
            <div v-else class="images-grid">
              <ImageCard
                v-for="image in images"
                :key="image.id"
                :image="image"
                :busy="loading || batchLoading || busyIds.has(image.id)"
                selectable
                :selected="selectedIds.includes(image.id)"
                :album-options="cardMoveOptions"
                @open="openDrawer(image)"
                @toggle="(isPublic) => handleToggle(image, isPublic)"
                @remove="handleRemove(image)"
                @select="(checked) => handleSelect(image, checked)"
                @move="(albumId) => handleMove(image, albumId)"
              />
            </div>
          </NSpin>
          <div class="images-pagination">
            <NPagination
              :page="page"
              :item-count="total"
              :page-size="size"
              @update:page="handlePageChange"
            />
          </div>
        </NTabPane>
        <NTabPane name="trash" :tab="t('user.trash.title')" display-directive="show:lazy">
          <TrashView />
        </NTabPane>
      </NTabs>
    </NCard>
    <ImageDetailDrawer v-model:show="drawerShow" :image="drawerImage" />
  </Page>
</template>

<style scoped>
.images-view__title {
  margin: 0 0 8px;
  font-size: 18px;
  font-weight: 600;
}
.images-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 12px;
}

.images-toolbar__filters {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.images-total {
  font-size: 13px;
  color: hsl(var(--muted-foreground));
}

.images-size {
  width: 130px;
}

.images-album-filter {
  width: 200px;
}

.images-batchbar {
  position: sticky;
  top: 0;
  z-index: 5;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 12px;
  padding: 8px 12px;
  border: 1px solid hsl(var(--border));
  border-radius: 6px;
  background: hsl(var(--muted) / 0.5);
}

.images-batchbar__count {
  font-size: 13px;
  font-weight: 600;
}

.images-batchbar__album {
  width: 180px;
}

.images-empty {
  padding: 48px 0;
}

.images-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(220px, 100%), 1fr));
  gap: 14px;
}

.images-pagination {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
