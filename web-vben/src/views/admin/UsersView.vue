<script setup lang="ts">
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
  NPagination,
  NSelect,
  NSwitch,
  NTag,
  useMessage,
  type DataTableColumns,
} from "naive-ui";
import { computed, h, onMounted, onUnmounted, reactive, ref } from "vue";
import {
  listGroups,
  listUsers,
  patchUser,
  type AdminUserView,
  type GroupView,
  type UserRole,
} from "@/api/admin";
import { formatBytes } from "@/lib/format";
import { formatDateTime } from "@/components/admin/format";

const { t, locale } = useI18n();
const message = useMessage();
const isMobile = useMediaQuery("(max-width: 640px)");

const PAGE_SIZE = 20;
const SEARCH_DEBOUNCE_MS = 500;

const ROLE_LABELS = computed<Record<UserRole, string>>(() => ({
  admin: t("admin.users.adminRole"),
  user: t("admin.users.userRole"),
}));

const users = ref<AdminUserView[]>([]);
const total = ref(0);
const page = ref(1);
const keyword = ref("");
const loading = ref(false);
const loadError = ref<unknown>(null);
/** 正在执行写操作（改状态/换组）的用户 id。 */
const busyIds = reactive(new Set<number>());

const groups = ref<GroupView[]>([]);
const groupOptions = computed(() =>
  groups.value.map((group) => ({ label: group.name, value: group.id })),
);

function groupLabel(groupId: number): string {
  return groups.value.find((group) => group.id === groupId)?.name ?? `#${groupId}`;
}

let listRequest = 0;
async function loadUsers(): Promise<void> {
  const request = ++listRequest;
  loading.value = true;
  loadError.value = null;
  try {
    const trimmed = keyword.value.trim();
    const data = await listUsers({
      page: page.value,
      size: PAGE_SIZE,
      keyword: trimmed === "" ? undefined : trimmed,
    });
    if (request !== listRequest) return;
    users.value = data.items;
    total.value = data.total;
    page.value = data.page;
  } catch (error) {
    if (request !== listRequest) return;
    loadError.value = error;
    users.value = [];
    total.value = 0;
  } finally {
    if (request === listRequest) loading.value = false;
  }
}

async function loadGroups(): Promise<void> {
  try {
    groups.value = await listGroups();
  } catch (error) {
    message.error(() => formatApiError(error, "admin.errors.groupsLoad"));
  }
}

onMounted(() => {
  void loadUsers();
  void loadGroups();
});

let searchTimer: ReturnType<typeof setTimeout> | null = null;

function clearSearchTimer(): void {
  if (searchTimer !== null) {
    clearTimeout(searchTimer);
    searchTimer = null;
  }
}

function searchNow(): void {
  clearSearchTimer();
  page.value = 1;
  void loadUsers();
}

/** 输入防抖触发搜索；清空关键词立即搜索；回车立即搜索（见模板）。 */
function handleKeywordInput(value: string): void {
  keyword.value = value;
  clearSearchTimer();
  if (value === "") {
    searchNow();
    return;
  }
  searchTimer = setTimeout(() => {
    searchTimer = null;
    page.value = 1;
    void loadUsers();
  }, SEARCH_DEBOUNCE_MS);
}

onUnmounted(() => {
  listRequest += 1;
  clearSearchTimer();
});

function handlePageChange(next: number): void {
  page.value = next;
  void loadUsers();
}

/** 启用/禁用：NSwitch 受控，失败时行数据不变、开关自动回滚。 */
async function handleStatusToggle(row: AdminUserView, enabled: boolean): Promise<void> {
  if (busyIds.has(row.id)) {
    return;
  }
  busyIds.add(row.id);
  try {
    const updated = await patchUser(row.id, {
      status: enabled ? "enabled" : "disabled",
    });
    users.value = users.value.map((user) => (user.id === updated.id ? updated : user));
    message.success(() =>
      enabled
        ? t("admin.users.enabled", { name: row.username })
        : t("admin.users.disabled", { name: row.username }),
    );
  } catch (error) {
    message.error(() => formatApiError(error, "admin.errors.userStatus"));
  } finally {
    busyIds.delete(row.id);
  }
}

/** 调整用户组：失败时行数据不变、下拉自动回滚。 */
async function handleGroupChange(row: AdminUserView, groupId: number): Promise<void> {
  if (busyIds.has(row.id) || groupId === row.group_id) {
    return;
  }
  busyIds.add(row.id);
  try {
    const updated = await patchUser(row.id, { group_id: groupId });
    users.value = users.value.map((user) => (user.id === updated.id ? updated : user));
    message.success(() =>
      t("admin.users.moved", { name: row.username, group: groupLabel(groupId) }),
    );
  } catch (error) {
    message.error(() => formatApiError(error, "admin.errors.userGroup"));
  } finally {
    busyIds.delete(row.id);
  }
}

const columns = computed<DataTableColumns<AdminUserView>>(() => [
  { title: t("admin.users.username"), key: "username", minWidth: 150, ellipsis: { tooltip: true } },
  { title: t("admin.users.email"), key: "email", minWidth: 220, ellipsis: { tooltip: true } },
  {
    title: t("admin.users.role"),
    key: "role",
    width: 125,
    render: (row) =>
      h(
        NTag,
        { size: "small", bordered: false, type: row.role === "admin" ? "warning" : "info" },
        { default: () => ROLE_LABELS.value[row.role] },
      ),
  },
  {
    title: t("admin.users.status"),
    key: "status",
    width: 80,
    render: (row) =>
      h(NSwitch, {
        size: "small",
        value: row.status === "enabled",
        loading: busyIds.has(row.id),
        disabled: busyIds.has(row.id),
        "aria-label": t("admin.users.toggleStatus", { name: row.username }),
        "onUpdate:value": (value: unknown) => {
          void handleStatusToggle(row, value === true);
        },
      }),
  },
  {
    title: t("admin.users.group"),
    key: "group_id",
    width: 170,
    render: (row) =>
      h(NSelect, {
        size: "small",
        value: row.group_id,
        options: groupOptions.value,
        loading: busyIds.has(row.id),
        disabled: busyIds.has(row.id),
        "aria-label": t("admin.users.groupLabel", { name: row.username }),
        style: "width: 150px",
        "onUpdate:value": (value: unknown) => {
          if (typeof value === "number") {
            void handleGroupChange(row, value);
          }
        },
      }),
  },
  {
    title: t("admin.users.usedBytes"),
    key: "used_bytes",
    width: 110,
    render: (row) => formatBytes(row.used_bytes),
  },
  {
    title: t("admin.users.createdAt"),
    key: "created_at",
    width: 190,
    render: (row) => formatDateTime(row.created_at, locale.value),
  },
]);
</script>

<template>
  <Page
    :title="t('admin.users.title')"
    :description="t('admin.users.description')"
    class="users-view min-w-0"
    header-class="flex-wrap items-center gap-4"
    content-class="min-w-0"
  >
    <NCard :bordered="false" class="min-w-0">
      <div class="users-toolbar">
        <span class="users-total">{{ t("admin.users.total", { count: total }) }}</span>
        <NInput
          class="users-search"
          :value="keyword"
          clearable
          :placeholder="t('admin.users.search')"
          @update:value="handleKeywordInput"
          @keyup.enter="searchNow"
        />
      </div>
      <NAlert v-if="loadError" type="error" class="users-alert">
        {{ formatApiError(loadError, "admin.errors.usersLoad") }}
        <NButton size="tiny" quaternary type="primary" @click="loadUsers">{{
          t("admin.common.retry")
        }}</NButton>
      </NAlert>
      <NDataTable
        :columns="columns"
        :data="users"
        :scroll-x="1045"
        :loading="loading"
        :row-key="(row: AdminUserView) => row.id"
      >
        <template #empty>
          <NEmpty :description="t('admin.users.empty')" />
        </template>
      </NDataTable>
      <div class="users-pagination">
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
.users-toolbar {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.users-total {
  font-size: 13px;
  color: hsl(var(--muted-foreground));
}

.users-search {
  width: 280px;
  max-width: 100%;
}

.users-alert {
  margin-bottom: 12px;
}

.users-pagination {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
