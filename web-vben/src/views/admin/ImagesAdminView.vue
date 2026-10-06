<script setup lang="ts">
import AdminThumbnail from "@/components/admin/AdminThumbnail.vue";
import { Page } from "@vben/common-ui";
import { useI18n } from "@vben/locales";
import { formatApiError } from "@/locales/errors";
import { useMediaQuery } from "@vueuse/core";
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NEmpty,
  NInput,
  NInputNumber,
  NPagination,
  NPopconfirm,
  NTag,
  useDialog,
  useMessage,
  type DataTableColumns,
  type DialogReactive,
} from "naive-ui";
import { computed, h, onBeforeUnmount, onMounted, ref, watch } from "vue";
import {
  deleteAdminImage,
  listAdminImages,
  purgeAllTrash,
  type AdminImageListParams,
} from "@/api/admin";
import type { ImageView } from "@/api/images";

import { formatDateTime } from "@/components/admin/format";
import { formatBytes } from "@/lib/format";

const { t, locale } = useI18n();
const message = useMessage();
const isMobile = useMediaQuery("(max-width: 640px)");
const dialog = useDialog();

const FILTER_DEBOUNCE_MS = 400;

const items = ref<ImageView[]>([]);
const total = ref(0);
const page = ref(1);
const size = ref(20);
const loading = ref(false);
const loadError = ref<unknown>(null);
const removingId = ref<number | null>(null);
const purging = ref(false);
const purgeConfirmation = ref<DialogReactive | null>(null);
watch(locale, () => {
  if (purgeConfirmation.value) {
    purgeConfirmation.value.positiveText = t("admin.images.purge");
    purgeConfirmation.value.negativeText = t("admin.common.cancel");
  }
});

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

let listRequest = 0;
async function load(): Promise<void> {
  const request = ++listRequest;
  loading.value = true;
  loadError.value = null;
  try {
    const data = await listAdminImages(currentParams());
    if (request !== listRequest) return;
    items.value = data.items;
    total.value = data.total;
    page.value = data.page;
  } catch (error) {
    if (request !== listRequest) return;
    loadError.value = error;
  } finally {
    if (request === listRequest) loading.value = false;
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
  listRequest += 1;
  purgeConfirmation.value?.destroy();
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

const columns = computed<DataTableColumns<ImageView>>(() => [
  {
    title: t("admin.policies.thumbnail"),
    key: "thumb",
    width: 90,
    render: (row) =>
      h(AdminThumbnail, { url: row.local_thumb_url, name: row.name, extension: extText(row) }),
  },
  { title: t("admin.common.name"), key: "name", minWidth: 200, ellipsis: { tooltip: true } },
  { title: t("admin.images.format"), key: "ext", width: 80, render: (row) => extText(row) },
  {
    title: t("admin.images.size"),
    key: "size",
    width: 100,
    render: (row) => formatBytes(row.size),
  },
  { title: t("admin.images.owner"), key: "user_id", width: 80 },
  {
    title: t("admin.images.visibility"),
    key: "is_public",
    width: 90,
    render: (row) =>
      h(
        NTag,
        { size: "small", bordered: false, type: row.is_public ? "success" : "default" },
        { default: () => (row.is_public ? t("admin.images.public") : t("admin.images.private")) },
      ),
  },
  {
    title: t("admin.images.createdAt"),
    key: "created_at",
    width: 190,
    render: (row) => formatDateTime(row.created_at, locale.value),
  },
  {
    title: t("admin.common.actions"),
    key: "actions",
    width: 120,
    render: (row) =>
      h(
        NPopconfirm,
        {
          positiveText: t("admin.images.confirmMove"),
          negativeText: t("admin.common.cancel"),
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
              { default: () => t("admin.images.moveToTrash") },
            ),
          default: () => t("admin.images.moveConfirm", { name: row.name }),
        },
      ),
  },
]);

async function removeImage(row: ImageView): Promise<void> {
  removingId.value = row.id;
  try {
    await deleteAdminImage(row.id);
    message.success(() => t("admin.images.moved", { name: row.name }));
    await load();
  } catch (error) {
    message.error(() => formatApiError(error, "admin.errors.delete"));
  } finally {
    removingId.value = null;
  }
}

function confirmPurgeAll(): void {
  if (purging.value || purgeConfirmation.value !== null) {
    return;
  }
  purgeConfirmation.value = dialog.warning({
    title: () => t("admin.images.emptyTrash"),
    content: () => t("admin.images.purgeConfirm"),
    positiveText: t("admin.images.purge"),
    negativeText: t("admin.common.cancel"),
    onPositiveClick: () => runPurgeAll(),
    onAfterLeave: () => {
      purgeConfirmation.value = null;
    },
  });
}

async function runPurgeAll(): Promise<void> {
  if (purging.value) return;
  purging.value = true;
  if (purgeConfirmation.value) {
    purgeConfirmation.value.closable = false;
    purgeConfirmation.value.closeOnEsc = false;
    purgeConfirmation.value.maskClosable = false;
    purgeConfirmation.value.negativeButtonProps = { disabled: true };
  }
  try {
    const result = await purgeAllTrash();
    message.success(() => t("admin.images.purged", { count: result.purged }));
    await load();
  } catch (error) {
    message.error(() => formatApiError(error, "admin.errors.purge"));
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
  <Page
    :title="t('admin.images.title')"
    :description="t('admin.images.description')"
    class="images-admin-view min-w-0"
    header-class="flex-wrap items-center gap-4"
    content-class="min-w-0"
  >
    <template #extra>
      <NButton type="error" size="small" secondary :loading="purging" @click="confirmPurgeAll">
        {{ t("admin.images.emptyTrash") }}
      </NButton>
    </template>
    <NCard :bordered="false" class="min-w-0">
      <div class="images-admin-toolbar">
        <NInputNumber
          class="images-admin-user"
          size="small"
          :value="filterUserId"
          :min="0"
          clearable
          :placeholder="t('admin.images.ownerPlaceholder')"
          @update:value="onUserIdChange"
        />
        <NInput
          class="images-admin-keyword"
          size="small"
          :value="filterKeyword"
          clearable
          :placeholder="t('admin.images.search')"
          @update:value="onKeywordInput"
          @keyup.enter="onKeywordEnter"
        />
        <span class="images-admin-total">{{ t("admin.images.total", { count: total }) }}</span>
      </div>
      <NAlert v-if="loadError" type="error" class="images-admin-alert">
        {{ formatApiError(loadError, "admin.errors.imagesLoad") }}
        <NButton size="tiny" quaternary type="primary" :disabled="loading" @click="load">
          {{ t("admin.common.retry") }}
        </NButton>
      </NAlert>
      <NDataTable
        :columns="columns"
        :data="items"
        :loading="loading"
        :row-key="rowKey"
        :scroll-x="1020"
        size="small"
      >
        <template #empty>
          <NEmpty :description="t('admin.images.empty')" />
        </template>
      </NDataTable>
      <div class="images-admin-pagination">
        <NPagination
          :page="page"
          :simple="isMobile"
          :item-count="total"
          :page-size="size"
          @update:page="handlePageChange"
        />
      </div>
    </NCard>
  </Page>
</template>

<style scoped>
.images-admin-alert {
  margin-bottom: 12px;
}
.images-admin-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.images-admin-user {
  width: 180px;
}

.images-admin-keyword {
  width: 320px;
  max-width: 100%;
}

.images-admin-total {
  margin-left: auto;
  font-size: 13px;
  color: hsl(var(--muted-foreground));
}

.images-admin-pagination {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
@media (max-width: 640px) {
  .images-admin-user,
  .images-admin-keyword {
    width: 100%;
  }

  .images-admin-total {
    margin-left: 0;
  }
}
</style>
