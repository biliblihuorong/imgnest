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
  NInputNumber,
  NModal,
  NPopconfirm,
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
import { computed, h, onMounted, reactive, ref } from "vue";
import { ApiError } from "@/api/client";
import {
  createGroup,
  deleteGroup,
  listGroups,
  listPolicies,
  updateGroup,
  type GroupInput,
  type GroupView,
  type PolicyView,
} from "@/api/admin";
import { formatBytesOrUnlimited, formatUploadPerMin } from "@/components/admin/format";

const { t } = useI18n();
const message = useMessage();
const isMobile = useMediaQuery("(max-width: 640px)");

const BYTES_PER_MB = 1024 * 1024;

const EXT_OPTIONS = ["jpg", "png", "gif", "webp", "bmp", "tif", "tiff", "heic", "avif"].map(
  (ext) => ({ label: ext, value: ext }),
);

const groups = ref<GroupView[]>([]);
const policies = ref<PolicyView[]>([]);
const loading = ref(false);
const loadError = ref<unknown>(null);
const deletingId = ref<number | null>(null);

async function loadGroups(): Promise<void> {
  loading.value = true;
  loadError.value = null;
  try {
    groups.value = await listGroups();
  } catch (error) {
    loadError.value = error;
  } finally {
    loading.value = false;
  }
}

async function loadPolicies(): Promise<void> {
  try {
    policies.value = await listPolicies();
  } catch (error) {
    message.error(() => formatApiError(error, "admin.errors.policiesLoad"));
  }
}

onMounted(() => {
  void loadGroups();
  void loadPolicies();
});

const policyOptions = computed(() =>
  policies.value.map((policy) => ({ label: policy.name, value: policy.id })),
);

function policyLabel(id: number): string {
  if (id === 0) {
    return "—";
  }
  return policies.value.find((policy) => policy.id === id)?.name ?? `#${id}`;
}

/** 删除：仍有成员时后端返回 30008，转成可理解的中文提示。 */
async function handleDelete(group: GroupView): Promise<void> {
  deletingId.value = group.id;
  try {
    await deleteGroup(group.id);
    message.success(() => t("admin.groups.deleted", { name: group.name }));
    await loadGroups();
  } catch (error) {
    if (error instanceof ApiError && error.code === 30008) {
      message.error(() => t("admin.groups.hasMembers"));
    } else {
      message.error(() => formatApiError(error, "admin.errors.delete"));
    }
  } finally {
    deletingId.value = null;
  }
}

/* ---------------- 新建/编辑表单 ---------------- */

interface GroupFormModel {
  name: string;
  /** MB 计的容量/单文件上限，提交时转字节；0 或空 = 不限。 */
  capacity_mb: number | null;
  max_file_mb: number | null;
  allowed_exts: string[];
  upload_per_min: number | null;
  default_policy_id: number | null;
  policy_ids: number[];
  is_default: boolean;
  is_guest: boolean;
}

function emptyModel(): GroupFormModel {
  return {
    name: "",
    capacity_mb: 0,
    max_file_mb: 0,
    allowed_exts: [],
    upload_per_min: 0,
    default_policy_id: null,
    policy_ids: [],
    is_default: false,
    is_guest: false,
  };
}

const formShow = ref(false);
const saving = ref(false);
const editingGroup = ref<GroupView | null>(null);
const formRef = ref<FormInst | null>(null);
const form = reactive<GroupFormModel>(emptyModel());

const formRules = computed<FormRules>(() => ({
  name: [
    {
      required: true,
      trigger: ["blur", "input"],
      renderMessage: () => t("admin.groups.nameRequired"),
      validator: (_rule: FormItemRule, value: string): boolean | Error =>
        value.trim().length > 0 ? true : new Error(t("admin.groups.nameRequired")),
    },
  ],
  default_policy_id: [
    {
      required: true,
      trigger: ["blur", "change"],
      renderMessage: () => t("admin.groups.policyRequired"),
      validator: (_rule: FormItemRule, value: number | null): boolean | Error =>
        value === null ? new Error(t("admin.groups.policyRequired")) : true,
    },
  ],
}));

function openCreate(): void {
  editingGroup.value = null;
  Object.assign(form, emptyModel());
  formShow.value = true;
}

function openEdit(group: GroupView): void {
  editingGroup.value = group;
  Object.assign(form, emptyModel(), {
    name: group.name,
    capacity_mb: group.capacity_bytes / BYTES_PER_MB,
    max_file_mb: group.max_file_bytes / BYTES_PER_MB,
    allowed_exts: [...group.allowed_exts],
    upload_per_min: group.upload_per_min,
    default_policy_id: group.default_policy_id === 0 ? null : group.default_policy_id,
    policy_ids: [...group.policy_ids],
    is_default: group.is_default,
    is_guest: group.is_guest,
  });
  formShow.value = true;
}

/** 表单模型 → API 请求体：MB 转字节，0 = 不限。 */
function buildPayload(): GroupInput {
  return {
    name: form.name.trim(),
    capacity_bytes: (form.capacity_mb ?? 0) * BYTES_PER_MB,
    max_file_bytes: (form.max_file_mb ?? 0) * BYTES_PER_MB,
    allowed_exts: [...form.allowed_exts],
    upload_per_min: form.upload_per_min ?? 0,
    default_policy_id: form.default_policy_id ?? 0,
    policy_ids: [...form.policy_ids],
    is_default: form.is_default,
    is_guest: form.is_guest,
  };
}

async function submitForm(): Promise<void> {
  if (saving.value) {
    return;
  }
  saving.value = true;
  try {
    await formRef.value?.validate();
  } catch {
    // 客户端校验未通过，错误已就地展示
    saving.value = false;
    return;
  }
  try {
    const payload = buildPayload();
    if (editingGroup.value === null) {
      await createGroup(payload);
      message.success(() => t("admin.groups.created", { name: payload.name }));
    } else {
      await updateGroup(editingGroup.value.id, payload);
      message.success(() => t("admin.groups.updated", { name: payload.name }));
    }
    formShow.value = false;
    await loadGroups();
  } catch (error) {
    message.error(() => formatApiError(error, "admin.errors.save"));
  } finally {
    saving.value = false;
  }
}

const columns = computed<DataTableColumns<GroupView>>(() => [
  { title: t("admin.common.name"), key: "name" },
  {
    title: t("admin.groups.default"),
    key: "is_default",
    width: 80,
    render: (row) =>
      row.is_default
        ? h(
            NTag,
            { size: "small", bordered: false, type: "info" },
            { default: () => t("admin.groups.default") },
          )
        : "—",
  },
  {
    title: t("admin.groups.guest"),
    key: "is_guest",
    width: 80,
    render: (row) =>
      row.is_guest
        ? h(
            NTag,
            { size: "small", bordered: false, type: "warning" },
            { default: () => t("admin.groups.guest") },
          )
        : "—",
  },
  {
    title: t("admin.groups.capacity"),
    key: "capacity_bytes",
    width: 110,
    render: (row) => formatBytesOrUnlimited(row.capacity_bytes, t),
  },
  {
    title: t("admin.groups.maxFile"),
    key: "max_file_bytes",
    width: 110,
    render: (row) => formatBytesOrUnlimited(row.max_file_bytes, t),
  },
  {
    title: t("admin.groups.uploadRate"),
    key: "upload_per_min",
    width: 100,
    render: (row) => formatUploadPerMin(row.upload_per_min, t),
  },
  { title: t("admin.groups.members"), key: "user_count", width: 80 },
  {
    title: t("admin.groups.defaultPolicy"),
    key: "default_policy_id",
    width: 130,
    render: (row) => policyLabel(row.default_policy_id),
  },
  {
    title: t("admin.common.actions"),
    key: "actions",
    width: 150,
    render: (row) =>
      h(
        NSpace,
        { size: "small", wrap: false },
        {
          default: () => [
            h(
              NButton,
              { size: "tiny", secondary: true, onClick: () => openEdit(row) },
              { default: () => t("admin.common.edit") },
            ),
            h(
              NPopconfirm,
              {
                to: false,
                positiveText: t("admin.common.confirmDelete"),
                negativeText: t("admin.common.cancel"),
                onPositiveClick: () => handleDelete(row),
              },
              {
                default: () => t("admin.groups.deleteConfirm", { name: row.name }),
                trigger: () =>
                  h(
                    NButton,
                    {
                      size: "tiny",
                      type: "error",
                      secondary: true,
                      loading: deletingId.value === row.id,
                    },
                    { default: () => t("admin.common.delete") },
                  ),
              },
            ),
          ],
        },
      ),
  },
]);
</script>

<template>
  <Page
    :title="t('admin.groups.title')"
    :description="t('admin.groups.description')"
    class="groups-view min-w-0"
    header-class="flex-wrap items-center gap-4"
    content-class="min-w-0"
  >
    <template #extra>
      <NButton type="primary" @click="openCreate">{{ t("admin.groups.create") }}</NButton>
    </template>
    <NCard :bordered="false" class="min-w-0">
      <div class="groups-toolbar">
        <span class="groups-total">{{ t("admin.groups.total", { count: groups.length }) }}</span>
      </div>
      <NAlert v-if="loadError" type="error" class="groups-alert">
        {{ formatApiError(loadError, "admin.errors.groupsLoad") }}
        <NButton size="tiny" quaternary type="primary" @click="loadGroups">{{
          t("admin.common.retry")
        }}</NButton>
      </NAlert>
      <NDataTable
        :columns="columns"
        :data="groups"
        :scroll-x="1040"
        :loading="loading"
        :row-key="(row: GroupView) => row.id"
      >
        <template #empty>
          <NEmpty :description="t('admin.groups.empty')" />
        </template>
      </NDataTable>
    </NCard>

    <NModal
      v-model:show="formShow"
      preset="card"
      :mask-closable="!saving"
      :close-on-esc="!saving"
      :closable="!saving"
      class="groups-modal"
      :title="
        editingGroup === null
          ? t('admin.groups.create')
          : t('admin.groups.edit', { name: editingGroup.name })
      "
    >
      <NForm
        ref="formRef"
        :model="form"
        :rules="formRules"
        :label-placement="isMobile ? 'top' : 'left'"
        :label-width="isMobile ? undefined : 110"
      >
        <NFormItem :label="t('admin.common.name')" path="name">
          <NInput
            v-model:value="form.name"
            :placeholder="t('admin.groups.name')"
            :disabled="saving"
          />
        </NFormItem>
        <NFormItem :label="t('admin.groups.capacityMb')" path="capacity_mb">
          <div class="field">
            <NInputNumber
              v-model:value="form.capacity_mb"
              class="field-control"
              :min="0"
              :placeholder="t('admin.groups.capacityPlaceholder')"
              :disabled="saving"
            />
            <p class="field-hint">{{ t("admin.groups.capacityHint") }}</p>
          </div>
        </NFormItem>
        <NFormItem :label="t('admin.groups.maxFileMb')" path="max_file_mb">
          <div class="field">
            <NInputNumber
              v-model:value="form.max_file_mb"
              class="field-control"
              :min="0"
              :placeholder="t('admin.groups.maxFilePlaceholder')"
              :disabled="saving"
            />
            <p class="field-hint">{{ t("admin.groups.maxFileHint") }}</p>
          </div>
        </NFormItem>
        <NFormItem :label="t('admin.groups.extensions')" path="allowed_exts">
          <div class="field">
            <NSelect
              v-model:value="form.allowed_exts"
              class="field-control"
              multiple
              filterable
              tag
              :options="EXT_OPTIONS"
              :placeholder="t('admin.groups.extensionsPlaceholder')"
              :disabled="saving"
            />
            <p class="field-hint">{{ t("admin.groups.extensionsHint") }}</p>
          </div>
        </NFormItem>
        <NFormItem :label="t('admin.groups.uploadRate')" path="upload_per_min">
          <div class="field">
            <NInputNumber
              v-model:value="form.upload_per_min"
              class="field-control"
              :min="0"
              :placeholder="t('admin.groups.ratePlaceholder')"
              :disabled="saving"
            />
            <p class="field-hint">{{ t("admin.groups.rateHint") }}</p>
          </div>
        </NFormItem>
        <NFormItem :label="t('admin.groups.defaultPolicy')" path="default_policy_id">
          <NSelect
            v-model:value="form.default_policy_id"
            :options="policyOptions"
            :placeholder="t('admin.groups.defaultPolicyPlaceholder')"
            clearable
            :disabled="saving"
          />
        </NFormItem>
        <NFormItem :label="t('admin.groups.policies')" path="policy_ids">
          <div class="field">
            <NSelect
              v-model:value="form.policy_ids"
              class="field-control"
              multiple
              :options="policyOptions"
              :placeholder="t('admin.groups.policiesPlaceholder')"
              :disabled="saving"
            />
            <p class="field-hint">{{ t("admin.groups.policiesHint") }}</p>
          </div>
        </NFormItem>
        <NFormItem :label="t('admin.groups.defaultGroup')" path="is_default">
          <NSwitch v-model:value="form.is_default" :disabled="saving" />
        </NFormItem>
        <NFormItem :label="t('admin.groups.guestGroup')" path="is_guest">
          <NSwitch v-model:value="form.is_guest" :disabled="saving" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton :disabled="saving" @click="formShow = false">{{
            t("admin.common.cancel")
          }}</NButton>
          <NButton type="primary" :loading="saving" @click="submitForm">{{
            t("admin.common.save")
          }}</NButton>
        </NSpace>
      </template>
    </NModal>
  </Page>
</template>

<style scoped>
.groups-modal {
  width: 600px;
  max-width: calc(100vw - 32px);
}

.groups-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.groups-total {
  font-size: 13px;
  color: hsl(var(--muted-foreground));
}

.groups-alert {
  margin-bottom: 12px;
}

.field {
  width: 100%;
}

.field-control {
  width: 100%;
}

.field-hint {
  margin: 4px 0 0;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}
</style>
