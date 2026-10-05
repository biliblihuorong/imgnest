<script setup lang="ts">
import { NButton, NEmpty, NPagination, NSpin, useMessage } from "naive-ui";
import { onMounted, reactive, ref } from "vue";
import { useRouter } from "vue-router";
import { deleteAlbum, listAlbums, type AlbumView } from "@/api/albums";
import AlbumCard from "@/components/albums/AlbumCard.vue";
import AlbumFormModal from "@/components/albums/AlbumFormModal.vue";

const PAGE_SIZE = 20;

const message = useMessage();
// 视图测试/匿名静态渲染等场景可能没有 router 实例
const router = useRouter();

const albums = ref<AlbumView[]>([]);
const total = ref(0);
const page = ref(1);
const loading = ref(false);
/** 正在执行写操作（删除）的相册 id。 */
const busyIds = reactive(new Set<number>());

const modalShow = ref(false);
const editing = ref<AlbumView | null>(null);

onMounted(() => {
  void load();
});

async function load(): Promise<void> {
  loading.value = true;
  try {
    const data = await listAlbums({ page: page.value, size: PAGE_SIZE });
    albums.value = data.items;
    total.value = data.total;
    page.value = data.page;
  } catch (error) {
    message.error(`相册列表加载失败：${error instanceof Error ? error.message : "未知错误"}`);
  } finally {
    loading.value = false;
  }
}

function handlePageChange(next: number): void {
  page.value = next;
  void load();
}

function openCreate(): void {
  editing.value = null;
  modalShow.value = true;
}

function openEdit(album: AlbumView): void {
  editing.value = album;
  modalShow.value = true;
}

function handleSaved(): void {
  void load();
}

async function handleRemove(album: AlbumView): Promise<void> {
  busyIds.add(album.id);
  try {
    await deleteAlbum(album.id);
    message.success("相册已删除，相册内图片已保留");
    await load();
  } catch (error) {
    message.error(`删除失败：${error instanceof Error ? error.message : "未知错误"}`);
  } finally {
    busyIds.delete(album.id);
  }
}

function goUpload(): void {
  void router?.push("/upload");
}
</script>

<template>
  <section class="albums-view">
    <div class="albums-view__toolbar">
      <span class="albums-view__total">共 {{ total }} 个相册</span>
      <NButton type="primary" size="small" @click="openCreate">新建相册</NButton>
    </div>
    <NSpin :show="loading">
      <NEmpty v-if="!loading && albums.length === 0" class="albums-view__empty" description="还没有相册">
        <template #extra>
          <NButton size="small" @click="goUpload">去上传页</NButton>
        </template>
      </NEmpty>
      <div v-else class="albums-view__grid">
        <AlbumCard
          v-for="album in albums"
          :key="album.id"
          :album="album"
          :busy="busyIds.has(album.id)"
          @edit="openEdit(album)"
          @remove="handleRemove(album)"
        />
      </div>
    </NSpin>
    <div class="albums-view__pagination">
      <NPagination
        :page="page"
        :item-count="total"
        :page-size="PAGE_SIZE"
        @update:page="handlePageChange"
      />
    </div>

    <AlbumFormModal v-model:show="modalShow" :album="editing" @saved="handleSaved" />
  </section>
</template>

<style scoped>
.albums-view {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.albums-view__toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}

.albums-view__total {
  font-size: 13px;
  color: rgba(128, 128, 128, 0.9);
}

.albums-view__empty {
  padding: 48px 0;
}

.albums-view__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 14px;
}

.albums-view__pagination {
  display: flex;
  justify-content: flex-end;
  margin-top: 4px;
}
</style>
