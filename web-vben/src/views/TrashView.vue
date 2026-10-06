<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { computed, h, onBeforeUnmount, onMounted, ref, watch } from "vue";
import {
  NAlert,
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
  type DialogReactive,
} from "naive-ui";
import {
  listTrash,
  purgeImages,
  restoreImages,
  type ImageBatchItem,
  type ImageView,
} from "@/api/images";
import { formatBytes } from "@/lib/format";
import { ApiError } from "@/api/client";
import { formatApiError } from "@/locales/errors";
import { formatDateTime, formatRetention } from "@/components/images/time";

const { t, locale } = useI18n();

const message = useMessage();
const dialog = useDialog();

let requestId = 0;
let active = true;
let confirmation: DialogReactive | null = null;
let pendingDialog: { ids: number[]; purge: boolean } | null = null;
const confirmOpen = ref(false);
function closeConfirmation() {
  confirmation?.destroy();
  confirmation = null;
  pendingDialog = null;
  confirmOpen.value = false;
}
onBeforeUnmount(() => {
  active = false;
  requestId++;
  closeConfirmation();
});
const sizeOptions = computed(() =>
  [20, 50, 100].map((count) => ({ label: t("user.common.perPage", { count }), value: count })),
);

const items = ref<ImageView[]>([]);
const total = ref(0);
const page = ref(1);
const size = ref(20);
const loading = ref(false);
const loadError = ref<unknown>(null);
const batchLoading = ref(false);
const checkedKeys = ref<number[]>([]);

const columns = computed<DataTableColumns<ImageView>>(() => [
  { type: "selection", disabled: () => batchLoading.value || loading.value },
  {
    title: t("user.common.name"),
    key: "name",
    ellipsis: { tooltip: true },
    render: (row) => `${row.name}（${row.ext.replace(/^\./, "").toUpperCase()}）`,
  },
  { title: t("user.common.size"), key: "size", width: 100, render: (row) => formatBytes(row.size) },
  {
    title: t("user.trash.deleted"),
    key: "deleted_at",
    width: 150,
    render: (row) => formatDateTime(row.deleted_at),
  },
  {
    title: t("user.trash.retention"),
    key: "purge_at",
    width: 130,
    render: (row) => formatRetention(row.purge_at),
  },
  {
    title: t("user.common.actions"),
    key: "actions",
    width: 190,
    render: (row) =>
      h(NSpace, { size: 4, wrap: false }, () => [
        h(
          NPopconfirm,
          {
            positiveText: t("user.trash.restore"),
            negativeText: t("user.common.cancel"),
            onPositiveClick: () => void runBatch(restoreImages, [row.id], "user.trash.restore"),
          },
          {
            trigger: () =>
              h(
                NButton,
                {
                  size: "tiny",
                  type: "primary",
                  quaternary: true,
                  disabled: batchLoading.value || loading.value,
                },
                () => t("user.trash.restore"),
              ),
            default: () => t("user.trash.restoreOne"),
          },
        ),
        h(
          NPopconfirm,
          {
            positiveText: t("user.trash.purge"),
            negativeText: t("user.common.cancel"),
            onPositiveClick: () => void runBatch(purgeImages, [row.id], "user.trash.purge"),
          },
          {
            trigger: () =>
              h(
                NButton,
                {
                  size: "tiny",
                  type: "error",
                  quaternary: true,
                  disabled: batchLoading.value || loading.value,
                },
                () => t("user.trash.purge"),
              ),
            default: () => t("user.trash.purgeOne"),
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
  const current = ++requestId;
  loading.value = true;
  loadError.value = null;
  try {
    const data = await listTrash({ page: page.value, size: size.value });
    if (!active || current !== requestId) return;
    items.value = data.items;
    checkedKeys.value = checkedKeys.value.filter((id) => data.items.some((item) => item.id === id));
    total.value = data.total;
    page.value = data.page;
  } catch (error) {
    if (active && current === requestId) {
      loadError.value = error;
      items.value = [];
      checkedKeys.value = [];
      total.value = 0;
    }
  } finally {
    if (current === requestId) loading.value = false;
  }
}

function handlePageChange(next: number): void {
  closeConfirmation();
  page.value = next;
  checkedKeys.value = [];
  void load();
}

function handleSizeChange(next: number): void {
  closeConfirmation();
  size.value = next;
  page.value = 1;
  checkedKeys.value = [];
  void load();
}

function handleCheckedKeysChange(keys: Array<string | number>): void {
  if (loading.value || batchLoading.value) return;
  checkedKeys.value = [...new Set(keys.map(Number))].filter((id) =>
    items.value.some((item) => item.id === id),
  );
}

function confirmationLabels(purge: boolean, count: number) {
  return {
    title: t(purge ? "user.trash.batchPurge" : "user.trash.batchRestore"),
    content: t(purge ? "user.trash.purgeConfirm" : "user.trash.restoreConfirm", { count }),
    positiveText: t(purge ? "user.trash.purge" : "user.trash.restore"),
    negativeText: t("user.common.cancel"),
  };
}
watch(locale, () => {
  if (confirmation && pendingDialog)
    Object.assign(confirmation, confirmationLabels(pendingDialog.purge, pendingDialog.ids.length));
});
function confirmBatchRestore() {
  openConfirmation(false);
}
function confirmBatchPurge() {
  openConfirmation(true);
}
function openConfirmation(purge: boolean) {
  const ids = [...checkedKeys.value];
  if (!ids.length || loading.value || batchLoading.value || confirmOpen.value) return;
  confirmOpen.value = true;
  pendingDialog = { ids, purge };
  confirmation = dialog.warning({
    ...confirmationLabels(purge, ids.length),
    onPositiveClick: () =>
      runBatch(
        purge ? purgeImages : restoreImages,
        ids,
        purge ? "user.trash.purge" : "user.trash.restore",
      ),
    onAfterLeave: () => {
      confirmation = null;
      pendingDialog = null;
      confirmOpen.value = false;
    },
  });
}

/** 执行恢复/彻底删除：207 逐项结果逐项反馈，操作后刷新并清空勾选。 */
async function runBatch(
  action: (ids: number[]) => Promise<ImageBatchItem[]>,
  ids: number[],
  label: string,
): Promise<void> {
  if (!active || batchLoading.value || loading.value || ids.length === 0) {
    return;
  }
  batchLoading.value = true;
  try {
    const results = await action(ids);
    if (!active) return;
    const failures = results.filter((item) => item.code !== 0);
    const successCount = results.length - failures.length;
    if (successCount > 0) {
      message.success(() =>
        t("user.common.batchSuccess", { action: t(label), count: successCount }),
      );
    }
    for (const item of failures) {
      message.error(() =>
        t("user.common.batchFailure", {
          action: t(label),
          id: item.id ?? t("user.common.unknown"),
          error: formatApiError(
            new ApiError(item.code, item.message, item.status),
            "user.common.loadError",
          ),
        }),
      );
    }
    await load();
    checkedKeys.value = [];
  } catch (error) {
    if (active)
      message.error(() =>
        t("user.common.actionFailure", {
          action: t(label),
          error: formatApiError(error, "user.common.loadError"),
        }),
      );
  } finally {
    batchLoading.value = false;
  }
}
</script>

<template>
  <section class="trash-view" :aria-label="t('user.trash.title')">
    <NAlert type="info" :show-icon="false">
      {{ t("user.trash.info") }}
    </NAlert>
    <div class="trash-toolbar">
      <NSpace>
        <NButton
          size="small"
          :disabled="checkedKeys.length === 0 || loading || confirmOpen || batchLoading"
          :loading="batchLoading"
          @click="confirmBatchRestore"
        >
          {{ t("user.trash.restoreSelected") }}
        </NButton>
        <NButton
          size="small"
          type="error"
          :disabled="checkedKeys.length === 0 || loading || confirmOpen || batchLoading"
          :loading="batchLoading"
          @click="confirmBatchPurge"
        >
          {{ t("user.trash.purgeSelected") }}
        </NButton>
      </NSpace>
      <NSpace align="center">
        <NSelect
          class="trash-size"
          size="small"
          :value="size"
          :options="sizeOptions"
          :aria-label="t('user.trash.pageSize')"
          @update:value="handleSizeChange"
        />
        <NButton
          size="small"
          quaternary
          :loading="loading"
          :disabled="loading || batchLoading"
          @click="load"
          >{{ t("user.common.refresh") }}</NButton
        >
      </NSpace>
    </div>

    <NAlert v-if="loadError" type="error">
      {{ formatApiError(loadError, "user.trash.loadError") }}
      <NButton text :disabled="loading" @click="load">{{ t("user.common.retry") }}</NButton>
    </NAlert>
    <NDataTable
      v-else
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
        <NEmpty :description="t('user.trash.empty')" />
      </template>
    </NDataTable>

    <div class="trash-pagination">
      <span class="trash-selection" :aria-label="t('user.trash.selectionLabel')" aria-live="polite">
        {{ t("user.trash.selection", { total, count: checkedKeys.length }) }}
      </span>
      <NPagination
        :page="page"
        :item-count="total"
        :page-size="size"
        :page-slot="5"
        @update:page="handlePageChange"
      />
    </div>
  </section>
</template>

<style scoped>
.trash-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
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
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  overflow-x: auto;
}

.trash-selection {
  font-size: 13px;
  color: hsl(var(--muted-foreground));
}

@media (max-width: 640px) {
  .trash-pagination {
    justify-content: center;
  }

  .trash-selection {
    width: 100%;
  }
}
</style>
