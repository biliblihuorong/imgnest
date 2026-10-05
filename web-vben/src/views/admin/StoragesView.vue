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

const { t } = useI18n();
const message = useMessage();
const isMobile = useMediaQuery("(max-width: 640px)");

/* ---------------- 存储列表 ---------------- */

const storages = ref<StorageView[]>([]);
const loading = ref(false);
const loadError = ref<unknown>(null);
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
    loadError.value = error;
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
  if (togglingId.value !== null) return;
  togglingId.value = row.id;
  try {
    const updated = await updateStorage(row.id, { enabled });
    storages.value = storages.value.map((item) => (item.id === updated.id ? updated : item));
  } catch (error) {
    message.error(() => formatApiError(error, "admin.errors.storageStatus"));
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
    message.error(() => formatApiError(error, "admin.errors.storageTest"));
  } finally {
    testingId.value = null;
  }
}

async function removeStorage(row: StorageView): Promise<void> {
  removingId.value = row.id;
  try {
    await deleteStorage(row.id);
    message.success(() => t("admin.storages.deleted", { name: row.name }));
    await loadStorages();
  } catch (error) {
    if (error instanceof ApiError && error.code === 30009) {
      message.error(() => t("admin.storages.referenced"));
    } else {
      message.error(() => formatApiError(error, "admin.errors.delete"));
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
    { label: t("admin.storages.put"), pass: result.checks.put },
    { label: t("admin.storages.copy"), pass: result.checks.copy },
    { label: t("admin.storages.remove"), pass: result.checks.delete },
  ];
});

const columns = computed<DataTableColumns<StorageView>>(() => [
  { title: t("admin.common.name"), key: "name", width: 160, ellipsis: { tooltip: true } },
  {
    title: t("admin.storages.driver"),
    key: "driver",
    width: 90,
    render: (row) =>
      h(
        NTag,
        { size: "small", bordered: false, type: row.driver === "local" ? "info" : "warning" },
        { default: () => row.driver },
      ),
  },
  { title: t("admin.storages.baseUrl"), key: "base_url", ellipsis: { tooltip: true } },
  {
    title: t("admin.common.enabled"),
    key: "enabled",
    width: 90,
    render: (row) =>
      h(NSwitch, {
        value: row.enabled,
        size: "small",
        loading: togglingId.value === row.id,
        disabled: togglingId.value !== null,
        "aria-label": t("admin.common.toggleStatus", { name: row.name }),
        onUpdateValue: (value: boolean) => void toggleEnabled(row, value),
      }),
  },
  {
    title: t("admin.common.actions"),
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
          { default: () => t("admin.storages.testConnection") },
        ),
        h(
          NButton,
          { size: "tiny", secondary: true, onClick: () => openEdit(row) },
          { default: () => t("admin.common.edit") },
        ),
        h(
          NPopconfirm,
          {
            positiveText: t("admin.common.confirmDelete"),
            negativeText: t("admin.common.cancel"),
            onPositiveClick: () => removeStorage(row),
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
                { default: () => t("admin.common.delete") },
              ),
            default: () => t("admin.storages.deleteConfirm", { name: row.name }),
          },
        ),
      ]),
  },
]);

/* ---------------- 新建/编辑存储 ---------------- */

type FormMode = "create" | "edit";

const showForm = ref(false);
const formMode = ref<FormMode>("create");
/** 编辑模式的目标存储（create 时为 null）。 */
const editing = ref<StorageView | null>(null);
const submitting = ref(false);
const formRef = ref<FormInst | null>(null);

const driverOptions = computed(() => [
  { label: t("admin.storages.localDriver"), value: "local" },
  { label: t("admin.storages.s3Driver"), value: "s3" },
]);

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

function requiredRule(key: string): FormItemRule {
  return {
    required: true,
    trigger: ["blur", "input"],
    renderMessage: () => t(key),
    validator: (_rule: FormItemRule, value: string): boolean | Error =>
      typeof value === "string" && value.trim().length > 0 ? true : new Error(t(key)),
  };
}

/** 仅在 driver=s3 时必填（region 等保持可选）。 */
function requiredIfS3(key: string): FormItemRule {
  return {
    required: true,
    trigger: ["blur", "input"],
    renderMessage: () => t(key),
    validator: (_rule: FormItemRule, value: string): boolean | Error => {
      if (form.driver !== "s3") {
        return true;
      }
      return typeof value === "string" && value.trim().length > 0 ? true : new Error(t(key));
    },
  };
}

const rules = computed<FormRules>(() => ({
  name: [requiredRule("admin.storages.nameRequired")],
  base_url: [requiredRule("admin.storages.baseUrlRequired")],
  endpoint: [requiredIfS3("admin.storages.endpointRequired")],
  bucket: [requiredIfS3("admin.storages.bucketRequired")],
  accessKeyId: [requiredIfS3("admin.storages.accessKeyRequired")],
  secretAccessKey: [requiredIfS3("admin.storages.secretKeyRequired")],
}));

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
  submitting.value = true;
  try {
    await formRef.value?.validate();
  } catch {
    // 客户端校验未通过，错误已就地展示
    submitting.value = false;
    return;
  }
  const isCreate = formMode.value === "create";
  try {
    if (isCreate) {
      const created = await createStorage(buildCreateBody());
      message.success(() => t("admin.storages.created", { name: created.name }));
    } else if (editing.value) {
      // 编辑仅提交 name/base_url；S3 config 不回显也不在此更新（换凭证需重建）
      const updated = await updateStorage(editing.value.id, {
        name: form.name.trim(),
        base_url: form.base_url.trim(),
      });
      message.success(() => t("admin.storages.saved", { name: updated.name }));
    }
    showForm.value = false;
    await loadStorages();
  } catch (error) {
    message.error(() =>
      formatApiError(error, isCreate ? "admin.errors.storageCreate" : "admin.errors.storageSave"),
    );
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
  <Page
    :title="t('admin.storages.title')"
    :description="t('admin.storages.description')"
    class="storages-view min-w-0"
    header-class="flex-wrap items-center gap-4"
    content-class="min-w-0"
  >
    <template #extra>
      <NButton type="primary" size="small" @click="openCreate">{{
        t("admin.storages.create")
      }}</NButton>
    </template>
    <NCard :bordered="false" class="min-w-0">
      <p class="storages-hint">
        {{ t("admin.storages.secretHint") }}
      </p>
      <NAlert v-if="loadError" type="error" class="storages-load-error">
        {{ formatApiError(loadError, "admin.errors.storagesLoad") }}
        <NButton size="tiny" quaternary @click="loadStorages">{{
          t("admin.common.retry")
        }}</NButton>
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
          <NEmpty :description="t('admin.storages.empty')" />
        </template>
      </NDataTable>
    </NCard>

    <NModal
      v-model:show="showForm"
      preset="card"
      :mask-closable="!submitting"
      :close-on-esc="!submitting"
      :closable="!submitting"
      class="storages-form-modal"
      :title="formMode === 'create' ? t('admin.storages.create') : t('admin.storages.edit')"
    >
      <NAlert
        v-if="formMode === 'edit' && editing?.driver === 's3'"
        type="warning"
        class="storages-form-alert"
      >
        {{ t("admin.storages.editSecretHint") }}
      </NAlert>
      <NForm
        ref="formRef"
        :model="form"
        :rules="rules"
        :label-placement="isMobile ? 'top' : 'left'"
        :label-width="isMobile ? undefined : 130"
      >
        <NFormItem :label="t('admin.common.name')" path="name">
          <NInput
            v-model:value="form.name"
            :placeholder="t('admin.storages.namePlaceholder')"
            :disabled="submitting"
          />
        </NFormItem>
        <NFormItem :label="t('admin.storages.driver')" path="driver">
          <NSelect
            v-model:value="form.driver"
            :options="driverOptions"
            :disabled="formMode === 'edit' || submitting"
          />
        </NFormItem>
        <NFormItem :label="t('admin.storages.baseUrl')" path="base_url">
          <NInput
            v-model:value="form.base_url"
            :placeholder="t('admin.storages.urlPlaceholder')"
            :disabled="submitting"
          />
        </NFormItem>
        <template v-if="formMode === 'create' && form.driver === 's3'">
          <NFormItem :label="t('admin.storages.endpoint')" path="endpoint">
            <NInput
              v-model:value="form.endpoint"
              :placeholder="t('admin.storages.endpointPlaceholder')"
              :disabled="submitting"
            />
          </NFormItem>
          <NFormItem :label="t('admin.storages.region')" path="region">
            <NInput
              v-model:value="form.region"
              :placeholder="t('admin.storages.regionPlaceholder')"
              :disabled="submitting"
            />
          </NFormItem>
          <NFormItem :label="t('admin.storages.bucket')" path="bucket">
            <NInput
              v-model:value="form.bucket"
              :placeholder="t('admin.storages.bucketPlaceholder')"
              :disabled="submitting"
            />
          </NFormItem>
          <NFormItem :label="t('admin.storages.accessKey')" path="accessKeyId">
            <NInput
              v-model:value="form.accessKeyId"
              :placeholder="t('admin.storages.accessKey')"
              :disabled="submitting"
            />
          </NFormItem>
          <NFormItem :label="t('admin.storages.secretKey')" path="secretAccessKey">
            <NInput
              v-model:value="form.secretAccessKey"
              type="password"
              show-password-on="click"
              :placeholder="t('admin.storages.secretKey')"
              :disabled="submitting"
            />
          </NFormItem>
          <NFormItem :label="t('admin.storages.pathStyle')" path="usePathStyle">
            <NSwitch v-model:value="form.usePathStyle" :disabled="submitting" />
          </NFormItem>
          <p class="storages-hint">
            {{ t("admin.storages.createSecretHint") }}
          </p>
        </template>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton :disabled="submitting" @click="showForm = false">{{
            t("admin.common.cancel")
          }}</NButton>
          <NButton type="primary" :loading="submitting" @click="submitForm">
            {{ formMode === "create" ? t("admin.common.create") : t("admin.common.save") }}
          </NButton>
        </NSpace>
      </template>
    </NModal>

    <NModal
      :show="testResult !== null"
      preset="card"
      :title="t('admin.storages.testResults')"
      class="storages-test-modal"
      @update:show="closeTestResult"
    >
      <p v-if="testTarget" class="storages-test-target">
        {{ t("admin.storages.testTarget", { name: testTarget.name, driver: testTarget.driver }) }}
      </p>
      <NAlert
        v-if="testResult"
        :type="testResult.ok ? 'success' : 'warning'"
        class="storages-test-summary"
      >
        {{ testResult.ok ? t("admin.storages.testPassed") : t("admin.storages.testFailed") }}
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
          <NButton type="primary" @click="closeTestResult(false)">{{
            t("admin.common.close")
          }}</NButton>
        </NSpace>
      </template>
    </NModal>
  </Page>
</template>

<style scoped>
.storages-hint {
  margin: 0 0 12px;
  font-size: 13px;
  color: hsl(var(--muted-foreground));
}

.storages-load-error {
  margin-bottom: 12px;
}

.storages-form-modal {
  width: 560px;
  max-width: calc(100vw - 32px);
}

.storages-form-alert {
  margin-bottom: 16px;
}

.storages-test-modal {
  width: 420px;
  max-width: calc(100vw - 32px);
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
