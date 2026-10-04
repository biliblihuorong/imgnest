<script setup lang="ts">
import { computed, h, onMounted, ref } from "vue";
import {
  NButton,
  NDataTable,
  NEmpty,
  NPagination,
  NPopconfirm,
  NSelect,
  NSpace,
  useDialog,
  useMessage,
  type DataTableColumns,
} from "naive-ui";
import {
  listTrash,
  purgeImages,
  restoreImages,
  type ImageBatchItem,
  type ImageView,
} from "@/api/images";
import { formatBytes } from "@/lib/format";
import { apiErrorMessage } from "@/components/images/error";
import { formatDateTime, formatRetention } from "@/components/images/time";

const message = useMessage();
const dialog = useDialog();

const sizeOptions = [
  { label: "每页 20 条", value: 20 },
  { label: "每页 50 条", value: 50 },
  { label: "每页 100 条", value: 100 },
];

const items = ref<ImageView[]>([]);
const total = ref(0);
const page = ref(1);
const size = ref(20);
const loading = ref(false);
const batchLoading = ref(false);
const checkedKeys = ref<number[]>([]);

const columns = computed<DataTableColumns<ImageView>>(() => [
  { type: "selection" },
  {
    title: "名称",
    key: "name",
    ellipsis: { tooltip: true },
    render: (row) => `${row.name}（${row.ext.replace(/^\./, "").toUpperCase()}）`,
  },
  { title: "大小", key: "size", width: 100, render: (row) => formatBytes(row.size) },
  {
    title: "删除时间",
    key: "deleted_at",
    width: 150,
    render: (row) => formatDateTime(row.deleted_at),
  },
  {
    title: "剩余保留",
    key: "purge_at",
    width: 130,
    render: (row) => formatRetention(row.purge_at),
  },
  {
    title: "操作",
    key: "actions",
    width: 190,
    render: (row) =>
      h(NSpace, { size: 4, wrap: false }, () => [
        h(
          NPopconfirm,
          {
            positiveText: "恢复",
            negativeText: "取消",
            onPositiveClick: () => void runBatch(restoreImages, [row.id], "恢复"),
          },
          {
            trigger: () =>
              h(NButton, { size: "tiny", type: "primary", quaternary: true }, () => "恢复"),
            default: () => "将该图片恢复到原路径？",
          },
        ),
        h(
          NPopconfirm,
          {
            positiveText: "彻底删除",
            negativeText: "取消",
            onPositiveClick: () => void runBatch(purgeImages, [row.id], "彻底删除"),
          },
          {
            trigger: () =>
              h(NButton, { size: "tiny", type: "error", quaternary: true }, () => "彻底删除"),
            default: () => "将永久移除该图片及其全部存储对象，不可恢复！确定继续吗？",
          },
        ),
      ]),
  },
]);

onMounted(() => {
  void load();
});

function rowKey(row: ImageView): number {
  return row.id;
}

async function load(): Promise<void> {
  loading.value = true;
  try {
    const data = await listTrash({ page: page.value, size: size.value });
    items.value = data.items;
    total.value = data.total;
    page.value = data.page;
  } catch (error) {
    message.error(`回收站加载失败：${apiErrorMessage(error)}`);
  } finally {
    loading.value = false;
  }
}

function handlePageChange(next: number): void {
  page.value = next;
  checkedKeys.value = [];
  void load();
}

function handleSizeChange(next: number): void {
  size.value = next;
  page.value = 1;
  checkedKeys.value = [];
  void load();
}

function handleCheckedKeysChange(keys: Array<string | number>): void {
  checkedKeys.value = keys.map(Number);
}

function confirmBatchRestore(): void {
  const ids = [...checkedKeys.value];
  if (ids.length === 0 || batchLoading.value) {
    return;
  }
  dialog.warning({
    title: "批量恢复",
    content: `将选中的 ${ids.length} 张图片恢复到原路径？`,
    positiveText: "恢复",
    negativeText: "取消",
    onPositiveClick: () => runBatch(restoreImages, ids, "恢复"),
  });
}

function confirmBatchPurge(): void {
  const ids = [...checkedKeys.value];
  if (ids.length === 0 || batchLoading.value) {
    return;
  }
  dialog.warning({
    title: "批量彻底删除",
    content: `将永久删除选中的 ${ids.length} 张图片及其全部存储对象，此操作不可恢复！`,
    positiveText: "彻底删除",
    negativeText: "取消",
    onPositiveClick: () => runBatch(purgeImages, ids, "彻底删除"),
  });
}

/** 执行恢复/彻底删除：207 逐项结果逐项反馈，操作后刷新并清空勾选。 */
async function runBatch(
  action: (ids: number[]) => Promise<ImageBatchItem[]>,
  ids: number[],
  label: string,
): Promise<void> {
  if (batchLoading.value) {
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
    checkedKeys.value = [];
  } catch (error) {
    message.error(`${label}失败：${apiErrorMessage(error)}`);
  } finally {
    batchLoading.value = false;
  }
}
</script>

<template>
  <section class="trash-view">
    <div class="trash-toolbar">
      <NSpace>
        <NButton
          size="small"
          :disabled="checkedKeys.length === 0"
          :loading="batchLoading"
          @click="confirmBatchRestore"
        >
          恢复选中
        </NButton>
        <NButton
          size="small"
          type="error"
          :disabled="checkedKeys.length === 0"
          :loading="batchLoading"
          @click="confirmBatchPurge"
        >
          彻底删除选中
        </NButton>
      </NSpace>
      <NSpace align="center">
        <NSelect
          class="trash-size"
          size="small"
          :value="size"
          :options="sizeOptions"
          @update:value="handleSizeChange"
        />
        <NButton size="small" quaternary @click="load">刷新</NButton>
      </NSpace>
    </div>

    <NDataTable
      :checked-row-keys="checkedKeys"
      :columns="columns"
      :data="items"
      :loading="loading"
      :row-key="rowKey"
      :scroll-x="720"
      size="small"
      @update:checked-row-keys="handleCheckedKeysChange"
    >
      <template #empty>
        <NEmpty description="回收站为空" />
      </template>
    </NDataTable>

    <div class="trash-pagination">
      <NPagination
        :page="page"
        :item-count="total"
        :page-size="size"
        @update:page="handlePageChange"
      />
    </div>
  </section>
</template>

<style scoped>
.trash-view {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.trash-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.trash-size {
  width: 130px;
}

.trash-pagination {
  display: flex;
  justify-content: flex-end;
}
</style>
