<script setup lang="ts">
import { Page } from "@vben/common-ui";
import { useI18n } from "@vben/locales";
import { useMediaQuery } from "@vueuse/core";
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NEmpty,
  NInputNumber,
  NPagination,
  NSwitch,
  NTag,
  NText,
  type DataTableColumns,
} from "naive-ui";
import { computed, h, onMounted, ref } from "vue";
import { listAuditLogs, type AuditEntry } from "@/api/audit";
import { formatDateTime } from "@/components/admin/format";
import { formatApiError } from "@/locales/errors";
import { auditActionKey } from "./auditActions";

const { t, locale } = useI18n();
const isMobile = useMediaQuery("(max-width: 640px)");

const PAGE_SIZE = 50;

const entries = ref<AuditEntry[]>([]);
const total = ref(0);
const page = ref(1);
const failedOnly = ref(false);
const userId = ref<number | null>(null);
const loading = ref(false);
const loadError = ref<unknown>(null);

let listRequest = 0;
async function load(): Promise<void> {
  const current = ++listRequest;
  loading.value = true;
  loadError.value = null;
  try {
    const data = await listAuditLogs({
      page: page.value,
      size: PAGE_SIZE,
      userId: userId.value ?? undefined,
      failed: failedOnly.value,
    });
    if (current !== listRequest) return;
    entries.value = data.items;
    total.value = data.total;
  } catch (error) {
    if (current !== listRequest) return;
    loadError.value = error;
    entries.value = [];
    total.value = 0;
  } finally {
    if (current === listRequest) loading.value = false;
  }
}

onMounted(() => void load());

function reload(): void {
  page.value = 1;
  void load();
}

function setFailedOnly(value: boolean): void {
  failedOnly.value = value;
  reload();
}

function setUser(value: number | null): void {
  userId.value = value && value > 0 ? value : null;
  reload();
}

function handlePageChange(value: number): void {
  page.value = value;
  void load();
}

function actionLabel(row: AuditEntry): string {
  const key = auditActionKey(row.method, row.route);
  return key ? t(`audit.actions.${key}`) : `${row.method} ${row.route}`;
}

function userLabel(row: AuditEntry): string {
  if (row.user_id === 0) return t("audit.anonymous");
  return row.user_name || t("audit.deletedUser", { id: row.user_id });
}

const columns = computed<DataTableColumns<AuditEntry>>(() => [
  {
    title: t("audit.columns.at"),
    key: "at",
    width: 150,
    render: (row) => formatDateTime(row.at, locale.value),
  },
  {
    title: t("audit.columns.user"),
    key: "user",
    width: 140,
    ellipsis: { tooltip: true },
    render: (row) =>
      row.user_id === 0
        ? h(NText, { depth: 3 }, { default: () => userLabel(row) })
        : h(
            NButton,
            { text: true, type: "primary", onClick: () => setUser(row.user_id) },
            { default: () => userLabel(row) },
          ),
  },
  {
    title: t("audit.columns.action"),
    key: "action",
    minWidth: 180,
    ellipsis: { tooltip: true },
    render: (row) => h("span", { title: `${row.method} ${row.route}` }, actionLabel(row)),
  },
  {
    title: t("audit.columns.target"),
    key: "target",
    width: 140,
    ellipsis: { tooltip: true },
    render: (row) => row.target || "-",
  },
  {
    title: t("audit.columns.status"),
    key: "status",
    width: 110,
    render: (row) =>
      h(
        NTag,
        { size: "small", bordered: false, type: row.status < 400 ? "success" : "error" },
        {
          default: () =>
            `${row.status < 400 ? t("audit.statusOk") : t("audit.statusFailed")} ${row.status}`,
        },
      ),
  },
  { title: t("audit.columns.ip"), key: "ip", width: 140, ellipsis: { tooltip: true } },
  {
    title: t("audit.columns.requestId"),
    key: "request_id",
    width: 220,
    ellipsis: { tooltip: true },
    render: (row) =>
      h("code", { class: "audit-request-id", title: t("audit.requestIdHint") }, row.request_id),
  },
]);
</script>

<template>
  <Page
    :title="t('audit.title')"
    :description="t('audit.description')"
    class="min-w-0"
    content-class="min-w-0"
  >
    <NCard :bordered="false" class="min-w-0">
      <div class="audit-toolbar">
        <span class="audit-total">{{ t("audit.total", { count: total }) }}</span>
        <div class="audit-filters">
          <NInputNumber
            class="audit-user"
            :value="userId"
            :min="1"
            :show-button="false"
            clearable
            :placeholder="t('audit.userFilter')"
            @update:value="setUser"
          />
          <label class="audit-failed">
            <NSwitch :value="failedOnly" @update:value="setFailedOnly" />
            {{ t("audit.failedOnly") }}
          </label>
        </div>
      </div>
      <NAlert v-if="loadError" type="error" class="audit-alert">
        {{ formatApiError(loadError, "audit.loadFailed") }}
        <NButton size="tiny" quaternary type="primary" @click="load">{{
          t("admin.common.retry")
        }}</NButton>
      </NAlert>
      <NDataTable
        :columns="columns"
        :data="entries"
        :scroll-x="1100"
        :loading="loading"
        :row-key="(row: AuditEntry) => row.id"
      >
        <template #empty>
          <NEmpty :description="t('audit.empty')" />
        </template>
      </NDataTable>
      <div class="audit-footer">
        <span class="audit-hint">{{ t("audit.retention") }}</span>
        <NPagination
          :page="page"
          :simple="isMobile"
          :item-count="total"
          :page-size="PAGE_SIZE"
          @update:page="handlePageChange"
        />
      </div>
    </NCard>
  </Page>
</template>

<style scoped>
.audit-toolbar {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.audit-filters {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 16px;
}

.audit-user {
  width: 180px;
}

.audit-failed {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}

.audit-total,
.audit-hint {
  font-size: 13px;
  color: hsl(var(--muted-foreground));
}

.audit-alert {
  margin-bottom: 12px;
}

.audit-footer {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-top: 16px;
}

:deep(.audit-request-id) {
  font-size: 12px;
}
</style>
