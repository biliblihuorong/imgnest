<script setup lang="ts">
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
import { apiErrorMessage, formatDateTime } from "@/components/admin/format";

const message = useMessage();

const PAGE_SIZE = 20;
const SEARCH_DEBOUNCE_MS = 500;

const ROLE_LABELS: Record<UserRole, string> = { admin: "管理员", user: "用户" };

const users = ref<AdminUserView[]>([]);
const total = ref(0);
const page = ref(1);
const keyword = ref("");
const loading = ref(false);
const loadError = ref<string | null>(null);
/** 正在执行写操作（改状态/换组）的用户 id。 */
const busyIds = reactive(new Set<number>());

const groups = ref<GroupView[]>([]);
const groupOptions = computed(() =>
  groups.value.map((group) => ({ label: group.name, value: group.id })),
);

function groupLabel(groupId: number): string {
  return groups.value.find((group) => group.id === groupId)?.name ?? `#${groupId}`;
}

async function loadUsers(): Promise<void> {
  loading.value = true;
  loadError.value = null;
  try {
    const trimmed = keyword.value.trim();
    const data = await listUsers({
      page: page.value,
      size: PAGE_SIZE,
      keyword: trimmed === "" ? undefined : trimmed,
    });
    users.value = data.items;
    total.value = data.total;
    page.value = data.page;
  } catch (error) {
    loadError.value = `用户列表加载失败：${apiErrorMessage(error)}`;
    users.value = [];
    total.value = 0;
  } finally {
    loading.value = false;
  }
}

async function loadGroups(): Promise<void> {
  try {
    groups.value = await listGroups();
  } catch (error) {
    message.error(`用户组列表加载失败：${apiErrorMessage(error)}`);
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

onUnmounted(clearSearchTimer);

function handlePageChange(next: number): void {
  page.value = next;
  void loadUsers();
}

/** 启用/禁用：NSwitch 受控，失败时行数据不变、开关自动回滚。 */
async function handleStatusToggle(row: AdminUserView, enabled: boolean): Promise<void> {
  busyIds.add(row.id);
  try {
    const updated = await patchUser(row.id, {
      status: enabled ? "enabled" : "disabled",
    });
    users.value = users.value.map((user) => (user.id === updated.id ? updated : user));
    message.success(enabled ? `已启用「${row.username}」` : `已禁用「${row.username}」`);
  } catch (error) {
    message.error(`状态修改失败：${apiErrorMessage(error)}`);
  } finally {
    busyIds.delete(row.id);
  }
}

/** 调整用户组：失败时行数据不变、下拉自动回滚。 */
async function handleGroupChange(row: AdminUserView, groupId: number): Promise<void> {
  if (groupId === row.group_id) {
    return;
  }
  busyIds.add(row.id);
  try {
    const updated = await patchUser(row.id, { group_id: groupId });
    users.value = users.value.map((user) => (user.id === updated.id ? updated : user));
    message.success(`已将「${row.username}」移至「${groupLabel(groupId)}」`);
  } catch (error) {
    message.error(`用户组调整失败：${apiErrorMessage(error)}`);
  } finally {
    busyIds.delete(row.id);
  }
}

const columns: DataTableColumns<AdminUserView> = [
  { title: "用户名", key: "username" },
  { title: "邮箱", key: "email" },
  {
    title: "角色",
    key: "role",
    width: 90,
    render: (row) =>
      h(
        NTag,
        { size: "small", bordered: false, type: row.role === "admin" ? "warning" : "info" },
        { default: () => ROLE_LABELS[row.role] },
      ),
  },
  {
    title: "状态",
    key: "status",
    width: 80,
    render: (row) =>
      h(NSwitch, {
        size: "small",
        value: row.status === "enabled",
        loading: busyIds.has(row.id),
        "onUpdate:value": (value: unknown) => {
          void handleStatusToggle(row, value === true);
        },
      }),
  },
  {
    title: "用户组",
    key: "group_id",
    width: 170,
    render: (row) =>
      h(NSelect, {
        size: "small",
        value: row.group_id,
        options: groupOptions.value,
        style: "width: 150px",
        "onUpdate:value": (value: unknown) => {
          if (typeof value === "number") {
            void handleGroupChange(row, value);
          }
        },
      }),
  },
  {
    title: "已用容量",
    key: "used_bytes",
    width: 110,
    render: (row) => formatBytes(row.used_bytes),
  },
  {
    title: "注册时间",
    key: "created_at",
    width: 150,
    render: (row) => formatDateTime(row.created_at),
  },
];
</script>

<template>
  <NCard title="用户管理" class="users-view">
    <div class="users-toolbar">
      <span class="users-total">共 {{ total }} 个用户</span>
      <NInput
        class="users-search"
        :value="keyword"
        clearable
        placeholder="搜索用户名或邮箱"
        @update:value="handleKeywordInput"
        @keyup.enter="searchNow"
      />
    </div>
    <NAlert v-if="loadError" type="error" class="users-alert">
      {{ loadError }}
      <NButton size="tiny" quaternary type="primary" @click="loadUsers">重试</NButton>
    </NAlert>
    <NDataTable
      :columns="columns"
      :data="users"
      :loading="loading"
      :row-key="(row: AdminUserView) => row.id"
    >
      <template #empty>
        <NEmpty description="暂无用户" />
      </template>
    </NDataTable>
    <div class="users-pagination">
      <NPagination
        :page="page"
        :item-count="total"
        :page-size="PAGE_SIZE"
        @update:page="handlePageChange"
      />
    </div>
  </NCard>
</template>

<style scoped>
.users-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.users-total {
  font-size: 13px;
  color: rgba(128, 128, 128, 0.9);
}

.users-search {
  width: 240px;
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
