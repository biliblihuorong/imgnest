<script setup lang="ts">
import {
  NButton,
  NCard,
  NDataTable,
  NEmpty,
  NImage,
  NInput,
  NInputNumber,
  NPagination,
  NPopconfirm,
  NTag,
  useDialog,
  useMessage,
  type DataTableColumns,
} from "naive-ui";
import { h, onBeforeUnmount, onMounted, ref } from "vue";
import {
  deleteAdminImage,
  listAdminImages,
  purgeAllTrash,
  type AdminImageListParams,
} from "@/api/admin";
import type { ImageView } from "@/api/images";
import { apiErrorMessage } from "@/components/images/error";
import { formatDateTime } from "@/components/images/time";
import { formatBytes } from "@/lib/format";

const message = useMessage();
const dialog = useDialog();

const FILTER_DEBOUNCE_MS = 400;

const items = ref<ImageView[]>([]);
const total = ref(0);
const page = ref(1);
const size = ref(20);
const loading = ref(false);
const removingId = ref<number | null>(null);
const purging = ref(false);

/** 过滤条件：属主 ID 与关键词（回车立即生效，输入防抖生效）。 */
const filterUserId = ref<number | null>(null);
const filterKeyword = ref("");

let debounceTimer: ReturnType<typeof setTimeout> | null = null;

function currentParams(): AdminImageListParams {
  const params: AdminImageListParams = { page: page.value, size: size.value };
  if (filterUserId.value !== null) {
    params.user_id = filterUserId.value;
  }
  const keyword = filterKeyword.value.trim();
  if (keyword) {
    params.keyword = keyword;
  }
  return params;
}

async function load(): Promise<void> {
  loading.value = true;
  try {
    const data = await listAdminImages(currentParams());
    items.value = data.items;
    total.value = data.total;
    page.value = data.page;
  } catch (error) {
    message.error(`图片列表加载失败：${apiErrorMessage(error)}`);
  } finally {
    loading.value = false;
  }
}

function searchFromFirstPage(): void {
  page.value = 1;
  void load();
}

function scheduleFilterSearch(): void {
  if (debounceTimer !== null) {
    clearTimeout(debounceTimer);
  }
  debounceTimer = setTimeout(() => {
    debounceTimer = null;
    searchFromFirstPage();
  }, FILTER_DEBOUNCE_MS);
}

function onUserIdChange(value: number | null): void {
  filterUserId.value = value;
  scheduleFilterSearch();
}

function onKeywordInput(value: string): void {
  filterKeyword.value = value;
  scheduleFilterSearch();
}

function onKeywordEnter(): void {
  if (debounceTimer !== null) {
    clearTimeout(debounceTimer);
    debounceTimer = null;
  }
  searchFromFirstPage();
}

onMounted(() => {
  void load();
});

onBeforeUnmount(() => {
  if (debounceTimer !== null) {
    clearTimeout(debounceTimer);
  }
});

function rowKey(row: ImageView): number {
  return row.id;
}

function extText(row: ImageView): string {
  return row.ext.replace(/^\./, "").toUpperCase() || "IMG";
}

const thumbFallbackStyle = {
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  width: "56px",
  height: "56px",
  borderRadius: "4px",
  fontSize: "12px",
  color: "rgba(128, 128, 128, 0.9)",
  background: "rgba(128, 128, 128, 0.15)",
} as const;

const columns: DataTableColumns<ImageView> = [
  {
    title: "缩略图",
    key: "thumb",
    width: 90,
    render: (row) =>
      row.local_thumb_url
        ? h(NImage, {
            src: row.local_thumb_url,
            objectFit: "cover",
            previewDisabled: true,
            imgProps: { alt: row.name },
            style: "width:56px;height:56px;border-radius:4px",
          })
        : h("span", { style: thumbFallbackStyle }, extText(row)),
  },
  { title: "名称", key: "name", minWidth: 200, ellipsis: { tooltip: true } },
  { title: "格式", key: "ext", width: 80, render: (row) => extText(row) },
  { title: "大小", key: "size", width: 100, render: (row) => formatBytes(row.size) },
  { title: "属主", key: "user_id", width: 80 },
  {
    title: "可见性",
    key: "is_public",
    width: 90,
    render: (row) =>
      h(
        NTag,
        { size: "small", bordered: false, type: row.is_public ? "success" : "default" },
        { default: () => (row.is_public ? "公开" : "私有") },
      ),
  },
  {
    title: "创建时间",
    key: "created_at",
    width: 150,
    render: (row) => formatDateTime(row.created_at),
  },
  {
    title: "操作",
    key: "actions",
    width: 120,
    render: (row) =>
      h(
        NPopconfirm,
        {
          positiveText: "确认移入",
          negativeText: "取消",
          onPositiveClick: () => removeImage(row),
        },
        {
          trigger: () =>
            h(
              NButton,
              {
                size: "tiny",
                type: "error",
                secondary: true,
                loading: removingId.value === row.id,
              },
              { default: () => "移入回收站" },
            ),
          default: () =>
            `将全站图片「${row.name}」移入回收站，原 URL 立即 404（保留期内属主可恢复）。确定继续吗？`,
        },
      ),
  },
];

async function removeImage(row: ImageView): Promise<void> {
  removingId.value = row.id;
  try {
    await deleteAdminImage(row.id);
    message.success(`图片「${row.name}」已移入回收站`);
    await load();
  } catch (error) {
    message.error(`删除失败：${apiErrorMessage(error)}`);
  } finally {
    removingId.value = null;
  }
}

function confirmPurgeAll(): void {
  if (purging.value) {
    return;
  }
  dialog.warning({
    title: "清空回收站",
    content: "将永久删除回收站中所有用户的图片及其存储对象，不可恢复。确定继续吗？",
    positiveText: "彻底清空",
    negativeText: "取消",
    onPositiveClick: () => runPurgeAll(),
  });
}

async function runPurgeAll(): Promise<void> {
  purging.value = true;
  try {
    const result = await purgeAllTrash();
    message.success(`已彻底清空回收站，共删除 ${result.purged} 张图片`);
    await load();
  } catch (error) {
    message.error(`清空回收站失败：${apiErrorMessage(error)}`);
  } finally {
    purging.value = false;
  }
}

function handlePageChange(next: number): void {
  page.value = next;
  void load();
}
</script>

<template>
  <section class="images-admin-view">
    <NCard title="全站图片">
      <template #header-extra>
        <NButton type="error" size="small" secondary :loading="purging" @click="confirmPurgeAll">
          清空回收站
        </NButton>
      </template>
      <div class="images-admin-toolbar">
        <NInputNumber
          class="images-admin-user"
          size="small"
          :value="filterUserId"
          :min="0"
          clearable
          placeholder="按属主用户 ID"
          @update:value="onUserIdChange"
        />
        <NInput
          class="images-admin-keyword"
          size="small"
          :value="filterKeyword"
          clearable
          placeholder="按文件名/路径搜索（回车立即搜索）"
          @update:value="onKeywordInput"
          @keyup.enter="onKeywordEnter"
        />
        <span class="images-admin-total">共 {{ total }} 张图片</span>
      </div>
      <NDataTable
        :columns="columns"
        :data="items"
        :loading="loading"
        :row-key="rowKey"
        :scroll-x="980"
        size="small"
      >
        <template #empty>
          <NEmpty description="没有符合条件的图片" />
        </template>
      </NDataTable>
      <div class="images-admin-pagination">
        <NPagination
          :page="page"
          :item-count="total"
          :page-size="size"
          @update:page="handlePageChange"
        />
      </div>
    </NCard>
  </section>
</template>

<style scoped>
.images-admin-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.images-admin-user {
  width: 180px;
}

.images-admin-keyword {
  width: 280px;
}

.images-admin-total {
  margin-left: auto;
  font-size: 13px;
  color: rgba(128, 128, 128, 0.9);
}

.images-admin-pagination {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
