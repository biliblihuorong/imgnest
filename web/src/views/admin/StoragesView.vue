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
import {
  createStorage,
  deleteStorage,
  listStorages,
  testStorage,
  updateStorage,
  type StorageDriver,
  type StorageInput,
  type StorageTestResult,
  type StorageView,
} from "@/api/admin";
import { ApiError } from "@/api/client";
import { apiErrorMessage } from "@/components/images/error";

const message = useMessage();

/* ---------------- 存储列表 ---------------- */

const storages = ref<StorageView[]>([]);
const loading = ref(false);
const loadError = ref<string | null>(null);
/** 正在切换启用状态的存储 id（防重入）。 */
const togglingId = ref<number | null>(null);
const removingId = ref<number | null>(null);
const testingId = ref<number | null>(null);

/** 最近一次连接测试的目标与结果（弹窗展示）。 */
const testTarget = ref<StorageView | null>(null);
const testResult = ref<StorageTestResult | null>(null);

async function loadStorages(): Promise<void> {
  loading.value = true;
  loadError.value = null;
  try {
    storages.value = await listStorages();
  } catch (error) {
    loadError.value = `存储列表加载失败：${apiErrorMessage(error)}`;
  } finally {
    loading.value = false;
  }
}

onMounted(() => {
  void loadStorages();
});

function rowKey(row: StorageView): number {
  return row.id;
}

/**
 * 切换启用状态。开关是受控组件（:value 绑定行数据）：
 * 失败时行数据不变，开关视觉自动回滚。
 */
async function toggleEnabled(row: StorageView, enabled: boolean): Promise<void> {
  togglingId.value = row.id;
  try {
    const updated = await updateStorage(row.id, { enabled });
    storages.value = storages.value.map((item) => (item.id === updated.id ? updated : item));
  } catch (error) {
    message.error(`启用状态修改失败：${apiErrorMessage(error)}`);
  } finally {
    togglingId.value = null;
  }
}

async function runTest(row: StorageView): Promise<void> {
  testingId.value = row.id;
  try {
    testResult.value = await testStorage(row.id);
    testTarget.value = row;
  } catch (error) {
    message.error(`连接测试失败：${apiErrorMessage(error)}`);
  } finally {
    testingId.value = null;
  }
}

async function removeStorage(row: StorageView): Promise<void> {
  removingId.value = row.id;
  try {
    await deleteStorage(row.id);
    message.success(`存储「${row.name}」已删除`);
    await loadStorages();
  } catch (error) {
    if (error instanceof ApiError && error.code === 30009) {
      message.error("该存储仍被上传规则引用，无法删除");
    } else {
      message.error(`删除失败：${apiErrorMessage(error)}`);
    }
  } finally {
    removingId.value = null;
  }
}

interface CheckItem {
  label: string;
  pass: boolean;
}

const testCheckItems = computed<CheckItem[]>(() => {
  const result = testResult.value;
  if (!result) {
    return [];
  }
  return [
    { label: "写入（put）", pass: result.checks.put },
    { label: "复制（copy）", pass: result.checks.copy },
    { label: "删除（delete）", pass: result.checks.delete },
  ];
});

const columns: DataTableColumns<StorageView> = [
  { title: "名称", key: "name", width: 160, ellipsis: { tooltip: true } },
  {
    title: "驱动",
    key: "driver",
    width: 90,
    render: (row) =>
      h(
        NTag,
        { size: "small", bordered: false, type: row.driver === "local" ? "info" : "warning" },
        { default: () => row.driver },
      ),
  },
  { title: "Base URL", key: "base_url", ellipsis: { tooltip: true } },
  {
    title: "启用",
    key: "enabled",
    width: 90,
    render: (row) =>
      h(NSwitch, {
        value: row.enabled,
        size: "small",
        loading: togglingId.value === row.id,
        "aria-label": `切换 ${row.name} 的启用状态`,
        onUpdateValue: (value: boolean) => void toggleEnabled(row, value),
      }),
  },
  {
    title: "操作",
    key: "actions",
    width: 230,
    render: (row) =>
      h(NSpace, { size: 4, wrap: false }, () => [
        h(
          NButton,
          {
            size: "tiny",
            type: "primary",
            secondary: true,
            loading: testingId.value === row.id,
            onClick: () => void runTest(row),
          },
          { default: () => "测试连接" },
        ),
        h(
          NButton,
          { size: "tiny", secondary: true, onClick: () => openEdit(row) },
          { default: () => "编辑" },
        ),
        h(
          NPopconfirm,
          {
            positiveText: "确认删除",
            negativeText: "取消",
            onPositiveClick: () => removeStorage(row),
          },
          {
            trigger: () =>
              h(
                NButton,
                { size: "tiny", type: "error", secondary: true, loading: removingId.value === row.id },
                { default: () => "删除" },
              ),
            default: () => `删除后不可恢复（引用该存储的规则会先被拒绝）。确定删除「${row.name}」吗？`,
          },
        ),
      ]),
  },
];

/* ---------------- 新建/编辑存储 ---------------- */

type FormMode = "create" | "edit";

const showForm = ref(false);
const formMode = ref<FormMode>("create");
/** 编辑模式的目标存储（create 时为 null）。 */
const editing = ref<StorageView | null>(null);
const submitting = ref(false);
const formRef = ref<FormInst | null>(null);

const driverOptions = [
  { label: "local（本地磁盘）", value: "local" },
  { label: "s3（S3 兼容对象存储）", value: "s3" },
];

const form = reactive({
  name: "",
  driver: "local" as StorageDriver,
  base_url: "",
  endpoint: "",
  region: "",
  bucket: "",
  accessKeyId: "",
  secretAccessKey: "",
  usePathStyle: false,
});

function resetForm(): void {
  form.name = "";
  form.driver = "local";
  form.base_url = "";
  form.endpoint = "";
  form.region = "";
  form.bucket = "";
  form.accessKeyId = "";
  form.secretAccessKey = "";
  form.usePathStyle = false;
}

function openCreate(): void {
  resetForm();
  formMode.value = "create";
  editing.value = null;
  showForm.value = true;
}

function openEdit(row: StorageView): void {
  resetForm();
  formMode.value = "edit";
  editing.value = row;
  form.name = row.name;
  form.driver = row.driver;
  form.base_url = row.base_url;
  showForm.value = true;
}

function requiredRule(text: string): FormItemRule {
  return {
    required: true,
    trigger: ["blur", "input"],
    validator: (_rule: FormItemRule, value: string): boolean | Error =>
      typeof value === "string" && value.trim().length > 0 ? true : new Error(text),
  };
}

/** 仅在 driver=s3 时必填（region 等保持可选）。 */
function requiredIfS3(text: string): FormItemRule {
  return {
    required: true,
    trigger: ["blur", "input"],
    validator: (_rule: FormItemRule, value: string): boolean | Error => {
      if (form.driver !== "s3") {
        return true;
      }
      return typeof value === "string" && value.trim().length > 0 ? true : new Error(text);
    },
  };
}

const rules: FormRules = {
  name: [requiredRule("请输入存储名称")],
  base_url: [requiredRule("请输入 Base URL")],
  endpoint: [requiredIfS3("请输入 Endpoint")],
  bucket: [requiredIfS3("请输入 Bucket")],
  accessKeyId: [requiredIfS3("请输入 Access Key ID")],
  secretAccessKey: [requiredIfS3("请输入 Secret Access Key")],
};

/** 组装创建请求体：config 仅在 s3 驱动下提交；local 不携带 config。 */
function buildCreateBody(): StorageInput {
  const body: StorageInput = {
    name: form.name.trim(),
    driver: form.driver,
    base_url: form.base_url.trim(),
  };
  if (form.driver === "s3") {
    body.config = {
      endpoint: form.endpoint.trim(),
      region: form.region.trim(),
      bucket: form.bucket.trim(),
      access_key_id: form.accessKeyId,
      secret_access_key: form.secretAccessKey,
      use_path_style: form.usePathStyle,
    };
  }
  return body;
}

async function submitForm(): Promise<void> {
  if (submitting.value) {
    return;
  }
  try {
    await formRef.value?.validate();
  } catch {
    // 客户端校验未通过，错误已就地展示
    return;
  }
  submitting.value = true;
  const isCreate = formMode.value === "create";
  try {
    if (isCreate) {
      const created = await createStorage(buildCreateBody());
      message.success(`存储「${created.name}」已创建`);
    } else if (editing.value) {
      // 编辑仅提交 name/base_url；S3 config 不回显也不在此更新（换凭证需重建）
      const updated = await updateStorage(editing.value.id, {
        name: form.name.trim(),
        base_url: form.base_url.trim(),
      });
      message.success(`存储「${updated.name}」已保存`);
    }
    showForm.value = false;
    await loadStorages();
  } catch (error) {
    const prefix = isCreate ? "创建存储失败" : "保存存储失败";
    message.error(`${prefix}：${apiErrorMessage(error)}`);
  } finally {
    submitting.value = false;
  }
}

function closeTestResult(show: boolean): void {
  if (!show) {
    testResult.value = null;
    testTarget.value = null;
  }
}
</script>

<template>
  <section class="storages-view">
    <NCard title="存储管理">
      <template #header-extra>
        <NButton type="primary" size="small" @click="openCreate">添加存储</NButton>
      </template>
      <p class="storages-hint">
        S3 密钥只在创建时提交（加密落库），密钥创建后不可查看；编辑仅修改名称与 Base URL。
      </p>
      <NAlert v-if="loadError" type="error" class="storages-load-error">
        {{ loadError }}
        <NButton size="tiny" quaternary @click="loadStorages">重试</NButton>
      </NAlert>
      <NDataTable
        :columns="columns"
        :data="storages"
        :loading="loading"
        :row-key="rowKey"
        :scroll-x="860"
        size="small"
      >
        <template #empty>
          <NEmpty description="暂无存储，点击右上角「添加存储」" />
        </template>
      </NDataTable>
    </NCard>

    <NModal
      v-model:show="showForm"
      preset="card"
      class="storages-form-modal"
      :title="formMode === 'create' ? '添加存储' : '编辑存储'"
    >
      <NAlert
        v-if="formMode === 'edit' && editing?.driver === 's3'"
        type="warning"
        class="storages-form-alert"
      >
        该存储为 S3 存储，密钥创建后不可查看、也不在此回显；如需更换凭证，请删除后重新创建。
      </NAlert>
      <NForm
        ref="formRef"
        :model="form"
        :rules="rules"
        label-placement="left"
        label-width="130"
      >
        <NFormItem label="名称" path="name">
          <NInput
            v-model:value="form.name"
            placeholder="例如：main"
            :disabled="submitting"
          />
        </NFormItem>
        <NFormItem label="驱动" path="driver">
          <NSelect
            v-model:value="form.driver"
            :options="driverOptions"
            :disabled="formMode === 'edit' || submitting"
          />
        </NFormItem>
        <NFormItem label="Base URL" path="base_url">
          <NInput
            v-model:value="form.base_url"
            placeholder="例如：https://cdn.example.com"
            :disabled="submitting"
          />
        </NFormItem>
        <template v-if="formMode === 'create' && form.driver === 's3'">
          <NFormItem label="Endpoint" path="endpoint">
            <NInput
              v-model:value="form.endpoint"
              placeholder="例如：https://s3.us-east-1.amazonaws.com"
              :disabled="submitting"
            />
          </NFormItem>
          <NFormItem label="Region" path="region">
            <NInput
              v-model:value="form.region"
              placeholder="例如：us-east-1（部分实现可留空）"
              :disabled="submitting"
            />
          </NFormItem>
          <NFormItem label="Bucket" path="bucket">
            <NInput
              v-model:value="form.bucket"
              placeholder="例如：imgnest"
              :disabled="submitting"
            />
          </NFormItem>
          <NFormItem label="Access Key ID" path="accessKeyId">
            <NInput
              v-model:value="form.accessKeyId"
              placeholder="Access Key ID"
              :disabled="submitting"
            />
          </NFormItem>
          <NFormItem label="Secret Access Key" path="secretAccessKey">
            <NInput
              v-model:value="form.secretAccessKey"
              type="password"
              show-password-on="click"
              placeholder="Secret Access Key"
              :disabled="submitting"
            />
          </NFormItem>
          <NFormItem label="Path Style" path="usePathStyle">
            <NSwitch v-model:value="form.usePathStyle" :disabled="submitting" />
          </NFormItem>
          <p class="storages-hint">
            S3 密钥加密存储，创建后不可查看；测试连接会依次验证写入/复制/删除能力。
          </p>
        </template>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton :disabled="submitting" @click="showForm = false">取消</NButton>
          <NButton type="primary" :loading="submitting" @click="submitForm">
            {{ formMode === "create" ? "创建" : "保存" }}
          </NButton>
        </NSpace>
      </template>
    </NModal>

    <NModal
      :show="testResult !== null"
      preset="card"
      title="连接测试结果"
      class="storages-test-modal"
      @update:show="closeTestResult"
    >
      <p v-if="testTarget" class="storages-test-target">
        存储「{{ testTarget.name }}」（{{ testTarget.driver }}）
      </p>
      <NAlert v-if="testResult" :type="testResult.ok ? 'success' : 'warning'" class="storages-test-summary">
        {{ testResult.ok ? "全部检查通过" : "存在未通过的检查项" }}
      </NAlert>
      <ul v-if="testResult" class="storages-check-list">
        <li v-for="item in testCheckItems" :key="item.label" class="storages-check-item">
          <NTag size="small" :bordered="false" :type="item.pass ? 'success' : 'error'">
            {{ item.pass ? "✓" : "✗" }}
          </NTag>
          <span>{{ item.label }}</span>
        </li>
      </ul>
      <template #footer>
        <NSpace justify="end">
          <NButton type="primary" @click="closeTestResult(false)">关闭</NButton>
        </NSpace>
      </template>
    </NModal>
  </section>
</template>

<style scoped>
.storages-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.storages-hint {
  margin: 0 0 12px;
  font-size: 13px;
  color: rgba(128, 128, 128, 0.9);
}

.storages-load-error {
  margin-bottom: 12px;
}

.storages-form-modal {
  width: 560px;
}

.storages-form-alert {
  margin-bottom: 16px;
}

.storages-test-modal {
  width: 420px;
}

.storages-test-target {
  margin: 0 0 12px;
  font-weight: 600;
}

.storages-test-summary {
  margin-bottom: 12px;
}

.storages-check-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.storages-check-item {
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>
