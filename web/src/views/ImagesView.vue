<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import {
  NEmpty,
  NPagination,
  NSelect,
  NSpin,
  NTabPane,
  NTabs,
  useMessage,
} from "naive-ui";
import TrashView from "./TrashView.vue";
import ImageCard from "@/components/images/ImageCard.vue";
import ImageDetailDrawer from "@/components/images/ImageDetailDrawer.vue";
import { apiErrorMessage } from "@/components/images/error";
import { deleteImage, listImages, setImageVisibility, type ImageView } from "@/api/images";

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

onMounted(() => {
  void load();
});

async function load(): Promise<void> {
  loading.value = true;
  try {
    const data = await listImages({ page: page.value, size: size.value });
    images.value = data.items;
    total.value = data.total;
    page.value = data.page;
  } catch (error) {
    message.error(`图片列表加载失败：${apiErrorMessage(error)}`);
  } finally {
    loading.value = false;
  }
}

function handlePageChange(next: number): void {
  page.value = next;
  void load();
}

function handleSizeChange(next: number): void {
  size.value = next;
  page.value = 1;
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
          <NSelect
            class="images-size"
            size="small"
            :value="size"
            :options="sizeOptions"
            @update:value="handleSizeChange"
          />
        </div>
        <NSpin :show="loading">
          <NEmpty
            v-if="!loading && images.length === 0"
            class="images-empty"
            description="还没有图片，去上传页传几张吧"
          />
          <div v-else class="images-grid">
            <ImageCard
              v-for="image in images"
              :key="image.id"
              :image="image"
              :busy="busyIds.has(image.id)"
              @open="openDrawer(image)"
              @toggle="(isPublic) => handleToggle(image, isPublic)"
              @remove="handleRemove(image)"
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

.images-total {
  font-size: 13px;
  color: rgba(128, 128, 128, 0.9);
}

.images-size {
  width: 130px;
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
