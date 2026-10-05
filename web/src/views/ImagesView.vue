<script setup lang="ts">
import { NButton, NEmpty, NPagination, NPopconfirm, NSelect, NSpin, NTabPane, NTabs, useMessage, type DropdownOption, type SelectOption } from "naive-ui";
import { computed, onMounted, reactive, ref } from "vue";
import TrashView from "./TrashView.vue";
import ImageCard from "@/components/images/ImageCard.vue";
import ImageDetailDrawer from "@/components/images/ImageDetailDrawer.vue";
import { apiErrorMessage } from "@/components/images/error";
import { listAlbums, type AlbumView } from "@/api/albums";
import {
  batchAlbums,
  batchDelete,
  batchPermission,
  deleteImage,
  listImages,
  setImageVisibility,
  type ImageBatchItem,
  type ImageView,
} from "@/api/images";

const message = useMessage();

const sizeOptions = [
  { label: "每页 20 条", value: 20 },
  { label: "每页 50 条", value: 50 },
  { label: "每页 100 条", value: 100 },
];

const images = ref<ImageView[]>([]);
const total = ref(0);
const page = ref(1);
const size = ref(20);
const loading = ref(false);
/** 正在执行写操作（改可见性/删除）的图片 id。 */
const busyIds = reactive(new Set<number>());
const drawerShow = ref(false);
const drawerImage = ref<ImageView | null>(null);

/** 相册列表（筛选与移动共用）；加载失败时降级为空列表。 */
const albums = ref<AlbumView[]>([]);
/** 相册筛选：null=全部（不传参）；0=未归类；正整数=指定相册。 */
const albumFilter = ref<number | null>(null);

/** 多选：选中图片 id（不跨页，翻页/筛选时清空）。 */
const selectedIds = ref<number[]>([]);
const batchLoading = ref(false);
/** 批量移动的目标相册；null=未选。 */
const batchTarget = ref<number | null>(null);

/** 筛选下拉：0=未归类 + 各相册（「全部」由 clearable 空值表达）。 */
const albumFilterOptions = computed<SelectOption[]>(() => [
  { label: "未归类", value: 0 },
  ...albums.value.map((album) => ({ label: album.name, value: album.id })),
]);

/** 批量移动的目标选项：0=移出相册 + 各相册。 */
const batchTargetOptions = computed<SelectOption[]>(() => [
  { label: "移出相册", value: 0 },
  ...albums.value.map((album) => ({ label: album.name, value: album.id })),
]);

/** 卡片「移动」下拉选项（key=相册 id，0=移出相册）。 */
const cardMoveOptions = computed<DropdownOption[]>(() => [
  { label: "移出相册", key: 0 },
  ...albums.value.map((album) => ({ label: album.name, key: album.id })),
]);

onMounted(() => {
  void load();
  void loadAlbums();
});

async function load(): Promise<void> {
  loading.value = true;
  try {
    const data = await listImages({
      page: page.value,
      size: size.value,
      album_id: albumFilter.value ?? undefined,
    });
    images.value = data.items;
    total.value = data.total;
    page.value = data.page;
  } catch (error) {
    message.error(`图片列表加载失败：${apiErrorMessage(error)}`);
  } finally {
    loading.value = false;
  }
}

async function loadAlbums(): Promise<void> {
  try {
    const data = await listAlbums({ page: 1, size: 100 });
    albums.value = data.items;
  } catch (error) {
    message.error(`相册列表加载失败：${apiErrorMessage(error)}`);
  }
}

function handlePageChange(next: number): void {
  page.value = next;
  clearSelection();
  void load();
}

function handleSizeChange(next: number): void {
  size.value = next;
  page.value = 1;
  clearSelection();
  void load();
}

function handleAlbumFilterChange(value: number | null): void {
  albumFilter.value = value;
  page.value = 1;
  clearSelection();
  void load();
}

/**
 * 切换公开/私有。开关是受控组件：失败时列表项不变，开关自动回滚。
 * 语义提示：私有只从公共列表/画廊隐藏，直链仍可访问。
 */
async function handleToggle(image: ImageView, isPublic: boolean): Promise<void> {
  busyIds.add(image.id);
  try {
    const updated = await setImageVisibility(image.id, isPublic);
    images.value = images.value.map((item) => (item.id === updated.id ? updated : item));
    message.success(isPublic ? "已设为公开" : "已设为私有，直链访问不受影响");
  } catch (error) {
    message.error(`可见性修改失败：${apiErrorMessage(error)}`);
  } finally {
    busyIds.delete(image.id);
  }
}

async function handleRemove(image: ImageView): Promise<void> {
  busyIds.add(image.id);
  try {
    await deleteImage(image.id);
    message.success("已移入回收站，保留期内可在回收站恢复");
    await load();
  } catch (error) {
    message.error(`删除失败：${apiErrorMessage(error)}`);
  } finally {
    busyIds.delete(image.id);
  }
}

/* ---------------- 多选批量（移动/删除/改公开私有） ---------------- */

function handleSelect(image: ImageView, checked: boolean): void {
  selectedIds.value = checked
    ? [...selectedIds.value, image.id]
    : selectedIds.value.filter((id) => id !== image.id);
}

function clearSelection(): void {
  selectedIds.value = [];
}

/**
 * 执行批量操作：207 逐项结果逐项反馈；成功后刷新列表。
 * 默认清空多选（卡片单图移动等保留选择的场景传 clearAfter: false）。
 */
async function runBatch(
  action: (ids: number[]) => Promise<ImageBatchItem[]>,
  ids: number[],
  label: string,
  options: { clearAfter?: boolean } = {},
): Promise<void> {
  if (ids.length === 0 || batchLoading.value) {
    return;
  }
  batchLoading.value = true;
  try {
    const results = await action(ids);
    const failures = results.filter((item) => item.code !== 0);
    const successCount = results.length - failures.length;
    if (successCount > 0) {
      message.success(`${label}成功 ${successCount} 张`);
    }
    for (const item of failures) {
      message.error(`${label}失败（ID ${item.id ?? "未知"}）：${item.message}`);
    }
    await load();
    if (options.clearAfter !== false) {
      clearSelection();
    }
  } catch (error) {
    message.error(`${label}失败：${apiErrorMessage(error)}`);
  } finally {
    batchLoading.value = false;
  }
}

function moveSelected(): void {
  const target = batchTarget.value;
  if (target === null) {
    message.warning("请先选择目标相册（选「移出相册」可批量移出）");
    return;
  }
  void runBatch((ids) => batchAlbums(ids, target), [...selectedIds.value], "移动");
}

function removeSelected(): void {
  void runBatch(batchDelete, [...selectedIds.value], "删除");
}

function batchVisibility(isPublic: boolean): void {
  void runBatch((ids) => batchPermission(ids, isPublic), [...selectedIds.value], isPublic ? "设为公开" : "设为私有");
}

function handleMove(image: ImageView, albumId: number): void {
  void runBatch((ids) => batchAlbums(ids, albumId), [image.id], "移动", { clearAfter: false });
}

function openDrawer(image: ImageView): void {
  drawerImage.value = image;
  drawerShow.value = true;
}
</script>

<template>
  <section class="images-view">
    <NTabs default-value="images" type="line">
      <NTabPane name="images" tab="图片">
        <div class="images-toolbar">
          <span class="images-total">共 {{ total }} 张图片</span>
          <div class="images-toolbar__filters">
            <NSelect
              class="images-size"
              size="small"
              :value="size"
              :options="sizeOptions"
              @update:value="handleSizeChange"
            />
            <NSelect
              class="images-album-filter"
              size="small"
              :value="albumFilter"
              :options="albumFilterOptions"
              placeholder="全部图片（按相册筛选）"
              clearable
              @update:value="handleAlbumFilterChange"
            />
          </div>
        </div>
        <div v-if="selectedIds.length > 0" class="images-batchbar">
          <span class="images-batchbar__count">已选 {{ selectedIds.length }} 张</span>
          <NSelect
            v-model:value="batchTarget"
            class="images-batchbar__album"
            size="small"
            :options="batchTargetOptions"
            placeholder="选择目标相册"
            :disabled="batchLoading"
          />
          <NButton size="small" type="primary" :loading="batchLoading" @click="moveSelected">
            移动到相册
          </NButton>
          <NPopconfirm
            positive-text="确认删除"
            negative-text="取消"
            @positive-click="removeSelected"
          >
            <template #trigger>
              <NButton size="small" type="error" :loading="batchLoading">批量删除</NButton>
            </template>
            将选中的 {{ selectedIds.length }} 张图片移入回收站？保留期内可恢复。
          </NPopconfirm>
          <NButton size="small" :disabled="batchLoading" @click="batchVisibility(true)">
            设为公开
          </NButton>
          <NButton size="small" :disabled="batchLoading" @click="batchVisibility(false)">
            设为私有
          </NButton>
          <NButton size="small" quaternary :disabled="batchLoading" @click="clearSelection">
            取消
          </NButton>
        </div>
        <NSpin :show="loading">
          <NEmpty
            v-if="!loading && images.length === 0"
            class="images-empty"
            :description="albumFilter === null ? '还没有图片，去上传页传几张吧' : '该相册还没有图片'"
          />
          <div v-else class="images-grid">
            <ImageCard
              v-for="image in images"
              :key="image.id"
              :image="image"
              :busy="busyIds.has(image.id)"
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
      <NTabPane name="trash" tab="回收站" display-directive="show:lazy">
        <TrashView />
      </NTabPane>
    </NTabs>

    <ImageDetailDrawer v-model:show="drawerShow" :image="drawerImage" />
  </section>
</template>

<style scoped>
.images-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.images-toolbar__filters {
  display: flex;
  align-items: center;
  gap: 8px;
}

.images-total {
  font-size: 13px;
  color: rgba(128, 128, 128, 0.9);
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
  border: 1px solid rgba(24, 160, 88, 0.35);
  border-radius: 6px;
  background: rgba(24, 160, 88, 0.06);
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
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 14px;
}

.images-pagination {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
