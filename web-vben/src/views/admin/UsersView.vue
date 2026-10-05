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
  NForm,
  NFormItem,
  NInput,
  NModal,
  NPagination,
  NSelect,
  NSpace,
  NSwitch,
  NTag,
  useMessage,
  type DataTableColumns,
  type FormInst,
  type FormItemRule,
  type FormRules,
} from "naive-ui";
import { computed, h, onMounted, onUnmounted, reactive, ref, shallowRef } from "vue";
import { useRouter } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import {
  createUser,
  listGroups,
  listUsers,
  patchUser,
  type AdminUserPatch,
  type AdminUserView,
  type GroupView,
  type UserRole,
  type UserStatus,
} from "@/api/admin";
import { formatBytes } from "@/lib/format";
import { formatDateTime } from "@/components/admin/format";

const { t, locale } = useI18n();
const message = useMessage();
const auth = useAuthStore();
const router = useRouter();
let disposed = false;
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
  groups.value
    .filter((group) => !group.is_guest)
    .map((group) => ({ label: group.name, value: group.id })),
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
  disposed = true;
  form.password = "";
  listRequest += 1;
  clearSearchTimer();
});

function handlePageChange(next: number): void {
  page.value = next;
  void loadUsers();
}

/** 避免旧请求覆盖新会话；服务端已吊销令牌时立即清理前端认证投影。 */
function captureSession(): () => boolean {
  const token = auth.token;
  const generation = auth.sessionGeneration;
  return () => token === auth.token && generation === auth.sessionGeneration;
}

function sessionFieldsChanged(before: AdminUserView, after: AdminUserView): boolean {
  return (
    before.username.trim() !== after.username.trim() ||
    before.email.trim().toLowerCase() !== after.email.trim().toLowerCase() ||
    before.role !== after.role ||
    before.status !== after.status ||
    before.group_id !== after.group_id
  );
}

async function applyUserUpdate(
  before: AdminUserView,
  updated: AdminUserView,
  isCurrentSession: () => boolean,
): Promise<boolean> {
  if (!isCurrentSession()) return false;
  if (auth.user?.id === updated.id) {
    if (sessionFieldsChanged(before, updated)) {
      listRequest += 1;
      clearSearchTimer();
      formShow.value = false;
      auth.clear();
      if (!disposed) message.success(() => t("admin.users.signInAgain"));
      await router.replace("/login");
      return false;
    }
    auth.setUser(updated);
  }
  if (disposed) return false;
  users.value = users.value.map((user) => (user.id === updated.id ? updated : user));
  return true;
}

/** 启用/禁用：NSwitch 受控，失败时行数据不变、开关自动回滚。 */
async function handleStatusToggle(row: AdminUserView, enabled: boolean): Promise<void> {
  if (busyIds.has(row.id) || enabled === (row.status === "enabled")) return;
  const isCurrentSession = captureSession();
  busyIds.add(row.id);
  try {
    const updated = await patchUser(row.id, {
      status: enabled ? "enabled" : "disabled",
    });
    if (!(await applyUserUpdate(row, updated, isCurrentSession))) return;
    message.success(() =>
      enabled
        ? t("admin.users.enabled", { name: row.username })
        : t("admin.users.disabled", { name: row.username }),
    );
  } catch (error) {
    if (!disposed && isCurrentSession()) {
      message.error(() => formatApiError(error, "admin.errors.userStatus"));
    }
  } finally {
    busyIds.delete(row.id);
  }
}

/** 调整用户组：失败时行数据不变、下拉自动回滚。 */
async function handleGroupChange(row: AdminUserView, groupId: number): Promise<void> {
  if (
    busyIds.has(row.id) ||
    groupId === row.group_id ||
    !groupOptions.value.some((group) => group.value === groupId)
  )
    return;
  const isCurrentSession = captureSession();
  busyIds.add(row.id);
  try {
    const updated = await patchUser(row.id, { group_id: groupId });
    if (!(await applyUserUpdate(row, updated, isCurrentSession))) return;
    message.success(() =>
      t("admin.users.moved", { name: row.username, group: groupLabel(groupId) }),
    );
  } catch (error) {
    if (!disposed && isCurrentSession()) {
      message.error(() => formatApiError(error, "admin.errors.userGroup"));
    }
  } finally {
    busyIds.delete(row.id);
  }
}

/* ---------------- 创建 / 编辑账户 ---------------- */

interface UserFormModel {
  username: string;
  email: string;
  password: string;
  display_name: string;
  role: UserRole;
  status: UserStatus;
  group_id: number | null;
}

function emptyModel(): UserFormModel {
  return {
    username: "",
    email: "",
    password: "",
    display_name: "",
    role: "user",
    status: "enabled",
    group_id: groups.value.find((group) => group.is_default && !group.is_guest)?.id ?? null,
  };
}

const formShow = shallowRef(false);
const saving = shallowRef(false);
const editingUser = shallowRef<AdminUserView | null>(null);
const formRef = ref<FormInst | null>(null);
const form = reactive<UserFormModel>(emptyModel());
const roleOptions = computed(() => [
  { label: ROLE_LABELS.value.user, value: "user" },
  { label: ROLE_LABELS.value.admin, value: "admin" },
]);
const statusOptions = computed(() => [
  { label: t("admin.common.enabled"), value: "enabled" },
  { label: t("admin.common.disabled"), value: "disabled" },
]);
const editingSelf = computed(
  () => editingUser.value?.id === auth.user?.id && editingUser.value !== null,
);

function validationRule<Value>(
  key: string,
  valid: (value: Value) => boolean,
  required = true,
): FormItemRule {
  return {
    required,
    trigger: ["blur", "input", "change"],
    renderMessage: () => t(key),
    validator: (_rule: FormItemRule, value: Value) => valid(value) || new Error(t(key)),
  };
}

const formRules = computed<FormRules>(() => ({
  username: validationRule("admin.users.usernameInvalid", (value: string) => {
    const length = Array.from(value.trim()).length;
    return length >= 3 && length <= 64;
  }),
  email: validationRule("admin.users.emailInvalid", (value: string) =>
    /^[^\s@<>]+@[^\s@<>]+$/u.test(value.trim()),
  ),
  password: validationRule("admin.users.passwordInvalid", (value: string) => {
    const bytes = new TextEncoder().encode(value).length;
    return editingUser.value !== null || (bytes >= 12 && bytes <= 72);
  }),
  display_name: validationRule(
    "admin.users.displayNameInvalid",
    (value: string) => Array.from(value.trim()).length <= 64 && !/\p{Cc}/u.test(value.trim()),
    false,
  ),
  role: validationRule(
    "admin.users.selectionInvalid",
    (value: string) => value === "admin" || value === "user",
  ),
  status: validationRule(
    "admin.users.selectionInvalid",
    (value: string) => value === "enabled" || value === "disabled",
  ),
  group_id: validationRule("admin.users.groupRequired", (value: number | null) =>
    groupOptions.value.some((group) => group.value === value),
  ),
}));

function openCreate(): void {
  if (saving.value) return;
  editingUser.value = null;
  Object.assign(form, emptyModel());
  formRef.value?.restoreValidation();
  formShow.value = true;
}

function openEdit(user: AdminUserView): void {
  if (saving.value || busyIds.has(user.id)) return;
  editingUser.value = { ...user };
  Object.assign(form, emptyModel(), {
    username: user.username,
    email: user.email,
    display_name: user.display_name,
    role: user.role,
    status: user.status,
    group_id: user.group_id,
  });
  formRef.value?.restoreValidation();
  formShow.value = true;
}

function closeForm(show = false): void {
  if (saving.value) return;
  formShow.value = show;
  if (!show) form.password = "";
}

async function submitForm(): Promise<void> {
  if (saving.value || !formRef.value) return;
  const isCurrentSession = captureSession();
  const before = editingUser.value;
  saving.value = true;
  try {
    await formRef.value.validate();
  } catch {
    saving.value = false;
    return;
  }
  if (disposed || !isCurrentSession()) {
    saving.value = false;
    return;
  }
  if (before) busyIds.add(before.id);
  try {
    const fields = {
      username: form.username.trim(),
      email: form.email.trim().toLowerCase(),
      display_name: form.display_name.trim(),
      role: form.role,
      status: form.status,
      group_id: form.group_id!,
    };
    let updated: AdminUserView;
    if (before) {
      // Send changed fields only: password and server-owned avatar/usage data never enter PATCH.
      const patch: AdminUserPatch = {};
      if (fields.username !== before.username.trim()) patch.username = fields.username;
      if (fields.email !== before.email.trim().toLowerCase()) patch.email = fields.email;
      if (fields.display_name !== before.display_name.trim())
        patch.display_name = fields.display_name;
      if (fields.role !== before.role) patch.role = fields.role;
      if (fields.status !== before.status) patch.status = fields.status;
      if (fields.group_id !== before.group_id) patch.group_id = fields.group_id;
      updated = await patchUser(before.id, patch);
      if (!(await applyUserUpdate(before, updated, isCurrentSession))) return;
    } else {
      updated = await createUser({ ...fields, password: form.password });
      form.password = "";
      if (disposed || !isCurrentSession()) return;
    }
    formShow.value = false;
    message.success(() =>
      t(before ? "admin.users.updated" : "admin.users.created", { name: updated.username }),
    );
    await loadUsers();
  } catch (error) {
    if (!disposed && isCurrentSession())
      message.error(() => formatApiError(error, "admin.errors.save"));
  } finally {
    saving.value = false;
    if (before) busyIds.delete(before.id);
  }
}

const columns = computed<DataTableColumns<AdminUserView>>(() => [
  { title: t("admin.users.username"), key: "username", minWidth: 150, ellipsis: { tooltip: true } },
  {
    title: t("admin.users.displayName"),
    key: "display_name",
    minWidth: 150,
    ellipsis: { tooltip: true },
    render: (row) => row.display_name || row.username,
  },
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
  {
    title: t("admin.common.actions"),
    key: "actions",
    width: 90,
    fixed: "right",
    render: (row) =>
      h(
        NButton,
        {
          size: "small",
          secondary: true,
          disabled: busyIds.has(row.id) || saving.value,
          "aria-label": t("admin.users.editLabel", { name: row.username }),
          onClick: () => openEdit(row),
        },
        { default: () => t("admin.common.edit") },
      ),
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
    <template #extra>
      <NButton type="primary" :disabled="saving" @click="openCreate">{{
        t("admin.users.create")
      }}</NButton>
    </template>
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
        :scroll-x="1285"
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

    <NModal
      :show="formShow"
      preset="card"
      class="users-modal"
      :mask-closable="!saving"
      :close-on-esc="!saving"
      :closable="!saving"
      :title="
        editingUser
          ? t('admin.users.edit', { name: editingUser.username })
          : t('admin.users.create')
      "
      @update:show="closeForm"
    >
      <NAlert v-if="editingSelf" type="warning" class="users-alert">
        {{ t("admin.users.selfEditHint") }}
      </NAlert>
      <NForm
        ref="formRef"
        :model="form"
        :rules="formRules"
        :label-placement="isMobile ? 'top' : 'left'"
        :label-width="isMobile ? undefined : 110"
        @submit.prevent="submitForm"
      >
        <NFormItem
          :label="t('admin.users.username')"
          path="username"
          label-for="admin-user-username"
        >
          <NInput
            v-model:value="form.username"
            :disabled="saving"
            :placeholder="t('admin.users.usernameHint')"
            :input-props="{ id: 'admin-user-username', autocomplete: 'off' }"
          />
        </NFormItem>
        <NFormItem :label="t('admin.users.email')" path="email" label-for="admin-user-email">
          <NInput
            v-model:value="form.email"
            :disabled="saving"
            :placeholder="t('admin.users.email')"
            :input-props="{ id: 'admin-user-email', inputmode: 'email', autocomplete: 'off' }"
          />
        </NFormItem>
        <NFormItem
          v-if="!editingUser"
          :label="t('admin.users.initialPassword')"
          path="password"
          label-for="admin-user-password"
        >
          <NInput
            v-model:value="form.password"
            type="password"
            show-password-on="click"
            :disabled="saving"
            :placeholder="t('admin.users.passwordInvalid')"
            :input-props="{ id: 'admin-user-password', autocomplete: 'new-password' }"
          />
        </NFormItem>
        <NFormItem
          :label="t('admin.users.displayName')"
          path="display_name"
          label-for="admin-user-display-name"
        >
          <div class="users-field">
            <NInput
              v-model:value="form.display_name"
              :disabled="saving"
              :placeholder="t('admin.users.displayName')"
              :input-props="{ id: 'admin-user-display-name', autocomplete: 'off' }"
            />
            <p class="users-field-hint">{{ t("admin.users.displayNameHint") }}</p>
          </div>
        </NFormItem>
        <NFormItem :label="t('admin.users.role')" path="role">
          <NSelect
            v-model:value="form.role"
            :options="roleOptions"
            :disabled="saving"
            :aria-label="t('admin.users.role')"
          />
        </NFormItem>
        <NFormItem :label="t('admin.users.status')" path="status">
          <NSelect
            v-model:value="form.status"
            :options="statusOptions"
            :disabled="saving"
            :aria-label="t('admin.users.status')"
          />
        </NFormItem>
        <NFormItem :label="t('admin.users.group')" path="group_id">
          <NSelect
            v-model:value="form.group_id"
            :options="groupOptions"
            :disabled="saving"
            :aria-label="t('admin.users.group')"
            :placeholder="t('admin.users.groupRequired')"
          />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton :disabled="saving" @click="closeForm()">{{ t("admin.common.cancel") }}</NButton>
          <NButton type="primary" :loading="saving" @click="submitForm">
            {{ t(editingUser ? "admin.common.save" : "admin.common.create") }}
          </NButton>
        </NSpace>
      </template>
    </NModal>
  </Page>
</template>

<style scoped>
.users-modal {
  width: 600px;
  max-width: calc(100vw - 32px);
}

.users-field {
  width: 100%;
}

.users-field-hint {
  margin: 4px 0 0;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}

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
