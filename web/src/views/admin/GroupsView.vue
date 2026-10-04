<script setup lang="ts">
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
import {
  apiErrorMessage,
  formatBytesOrUnlimited,
  formatUploadPerMin,
} from "@/components/admin/format";

const message = useMessage();

const BYTES_PER_MB = 1024 * 1024;

const EXT_OPTIONS = ["jpg", "png", "gif", "webp", "bmp", "tif", "tiff", "heic", "avif"].map(
  (ext) => ({ label: ext, value: ext }),
);

const groups = ref<GroupView[]>([]);
const policies = ref<PolicyView[]>([]);
const loading = ref(false);
const loadError = ref<string | null>(null);
const deletingId = ref<number | null>(null);

async function loadGroups(): Promise<void> {
  loading.value = true;
  loadError.value = null;
  try {
    groups.value = await listGroups();
  } catch (error) {
    loadError.value = `用户组列表加载失败：${apiErrorMessage(error)}`;
  } finally {
    loading.value = false;
  }
}

async function loadPolicies(): Promise<void> {
  try {
    policies.value = await listPolicies();
  } catch (error) {
    message.error(`策略列表加载失败：${apiErrorMessage(error)}`);
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
    message.success(`用户组「${group.name}」已删除`);
    await loadGroups();
  } catch (error) {
    if (error instanceof ApiError && error.code === 30008) {
      message.error("用户组仍有成员，无法删除");
    } else {
      message.error(`删除失败：${apiErrorMessage(error)}`);
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

const formRules: FormRules = {
  name: [
    {
      required: true,
      trigger: ["blur", "input"],
      validator: (_rule: FormItemRule, value: string): boolean | Error =>
        value.trim().length > 0 ? true : new Error("请输入用户组名称"),
    },
  ],
  default_policy_id: [
    {
      required: true,
      trigger: ["blur", "change"],
      validator: (_rule: FormItemRule, value: number | null): boolean | Error =>
        value === null ? new Error("请选择默认策略") : true,
    },
  ],
};

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
  try {
    await formRef.value?.validate();
  } catch {
    // 客户端校验未通过，错误已就地展示
    return;
  }
  saving.value = true;
  try {
    const payload = buildPayload();
    if (editingGroup.value === null) {
      await createGroup(payload);
      message.success(`用户组「${payload.name}」已创建`);
    } else {
      await updateGroup(editingGroup.value.id, payload);
      message.success(`用户组「${payload.name}」已更新`);
    }
    formShow.value = false;
    await loadGroups();
  } catch (error) {
    message.error(`保存失败：${apiErrorMessage(error)}`);
  } finally {
    saving.value = false;
  }
}

const columns: DataTableColumns<GroupView> = [
  { title: "名称", key: "name" },
  {
    title: "默认",
    key: "is_default",
    width: 80,
    render: (row) =>
      row.is_default
        ? h(NTag, { size: "small", bordered: false, type: "info" }, { default: () => "默认" })
        : "—",
  },
  {
    title: "游客",
    key: "is_guest",
    width: 80,
    render: (row) =>
      row.is_guest
        ? h(NTag, { size: "small", bordered: false, type: "warning" }, { default: () => "游客" })
        : "—",
  },
  {
    title: "容量上限",
    key: "capacity_bytes",
    width: 110,
    render: (row) => formatBytesOrUnlimited(row.capacity_bytes),
  },
  {
    title: "单文件上限",
    key: "max_file_bytes",
    width: 110,
    render: (row) => formatBytesOrUnlimited(row.max_file_bytes),
  },
  {
    title: "上传频率",
    key: "upload_per_min",
    width: 100,
    render: (row) => formatUploadPerMin(row.upload_per_min),
  },
  { title: "成员数", key: "user_count", width: 80 },
  {
    title: "默认策略",
    key: "default_policy_id",
    width: 130,
    render: (row) => policyLabel(row.default_policy_id),
  },
  {
    title: "操作",
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
              { default: () => "编辑" },
            ),
            h(
              NPopconfirm,
              {
                to: false,
                positiveText: "确认删除",
                negativeText: "取消",
                onPositiveClick: () => handleDelete(row),
              },
              {
                default: () => `删除用户组「${row.name}」？该操作不可恢复。`,
                trigger: () =>
                  h(
                    NButton,
                    {
                      size: "tiny",
                      type: "error",
                      secondary: true,
                      loading: deletingId.value === row.id,
                    },
                    { default: () => "删除" },
                  ),
              },
            ),
          ],
        },
      ),
  },
];
</script>

<template>
  <NCard title="用户组管理" class="groups-view">
    <div class="groups-toolbar">
      <span class="groups-total">共 {{ groups.length }} 个用户组</span>
      <NButton type="primary" @click="openCreate">新建用户组</NButton>
    </div>
    <NAlert v-if="loadError" type="error" class="groups-alert">
      {{ loadError }}
      <NButton size="tiny" quaternary type="primary" @click="loadGroups">重试</NButton>
    </NAlert>
    <NDataTable
      :columns="columns"
      :data="groups"
      :loading="loading"
      :row-key="(row: GroupView) => row.id"
    >
      <template #empty>
        <NEmpty description="暂无用户组" />
      </template>
    </NDataTable>

    <NModal
      v-model:show="formShow"
      preset="card"
      class="groups-modal"
      style="width: 600px"
      :title="editingGroup === null ? '新建用户组' : `编辑用户组：${editingGroup.name}`"
    >
      <NForm
        ref="formRef"
        :model="form"
        :rules="formRules"
        label-placement="left"
        :label-width="110"
      >
        <NFormItem label="名称" path="name">
          <NInput v-model:value="form.name" placeholder="用户组名称" :disabled="saving" />
        </NFormItem>
        <NFormItem label="容量（MB）" path="capacity_mb">
          <div class="field">
            <NInputNumber
              v-model:value="form.capacity_mb"
              class="field-control"
              :min="0"
              placeholder="容量上限（MB），0 表示不限"
              :disabled="saving"
            />
            <p class="field-hint">组内每个用户的总容量上限，0 表示不限。</p>
          </div>
        </NFormItem>
        <NFormItem label="单文件（MB）" path="max_file_mb">
          <div class="field">
            <NInputNumber
              v-model:value="form.max_file_mb"
              class="field-control"
              :min="0"
              placeholder="单文件上限（MB），0 表示不限"
              :disabled="saving"
            />
            <p class="field-hint">单个上传文件的大小上限，0 表示不限。</p>
          </div>
        </NFormItem>
        <NFormItem label="允许扩展名" path="allowed_exts">
          <div class="field">
            <NSelect
              v-model:value="form.allowed_exts"
              class="field-control"
              multiple
              filterable
              tag
              :options="EXT_OPTIONS"
              placeholder="可多选，也可输入自定义扩展名"
              :disabled="saving"
            />
            <p class="field-hint">留空表示不限扩展名；可直接输入内置选项之外的扩展名。</p>
          </div>
        </NFormItem>
        <NFormItem label="上传频率" path="upload_per_min">
          <div class="field">
            <NInputNumber
              v-model:value="form.upload_per_min"
              class="field-control"
              :min="0"
              placeholder="每分钟上传次数上限，0 表示不限"
              :disabled="saving"
            />
            <p class="field-hint">每分钟允许的上传次数（游客限流同样取该值），0 表示不限。</p>
          </div>
        </NFormItem>
        <NFormItem label="默认策略" path="default_policy_id">
          <NSelect
            v-model:value="form.default_policy_id"
            :options="policyOptions"
            placeholder="默认存储策略"
            clearable
            :disabled="saving"
          />
        </NFormItem>
        <NFormItem label="可用策略" path="policy_ids">
          <div class="field">
            <NSelect
              v-model:value="form.policy_ids"
              class="field-control"
              multiple
              :options="policyOptions"
              placeholder="可用策略（可多选）"
              :disabled="saving"
            />
            <p class="field-hint">该组成员上传时可选择的规则集合。</p>
          </div>
        </NFormItem>
        <NFormItem label="默认组" path="is_default">
          <NSwitch v-model:value="form.is_default" :disabled="saving" />
        </NFormItem>
        <NFormItem label="游客组" path="is_guest">
          <NSwitch v-model:value="form.is_guest" :disabled="saving" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton :disabled="saving" @click="formShow = false">取消</NButton>
          <NButton type="primary" :loading="saving" @click="submitForm">保存</NButton>
        </NSpace>
      </template>
    </NModal>
  </NCard>
</template>

<style scoped>
.groups-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.groups-total {
  font-size: 13px;
  color: rgba(128, 128, 128, 0.9);
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
  color: rgba(128, 128, 128, 0.85);
}
</style>
