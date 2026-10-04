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
  NTabPane,
  NTabs,
  NTag,
  useMessage,
  type DataTableColumns,
  type FormInst,
  type FormItemRule,
  type FormRules,
} from "naive-ui";
import { computed, h, onMounted, reactive, ref } from "vue";
import {
  createPolicy,
  deletePolicy,
  listPolicies,
  listStorages,
  previewPolicy,
  updatePolicy,
  type HEIFMode,
  type LinkPrefer,
  type OnConflict,
  type PolicyInput,
  type PolicyPreviewResult,
  type PolicyView,
  type ScrubMode,
  type StorageView,
  type WebPMode,
} from "@/api/admin";
import { apiErrorMessage } from "@/components/images/error";

const message = useMessage();

/* ---------------- 列表 ---------------- */

const policies = ref<PolicyView[]>([]);
const storages = ref<StorageView[]>([]);
const loading = ref(false);
const loadError = ref<string | null>(null);
const removingId = ref<number | null>(null);

async function loadAll(): Promise<void> {
  loading.value = true;
  loadError.value = null;
  try {
    const [policyList, storageList] = await Promise.all([listPolicies(), listStorages()]);
    policies.value = policyList;
    storages.value = storageList;
  } catch (error) {
    loadError.value = `规则列表加载失败：${apiErrorMessage(error)}`;
  } finally {
    loading.value = false;
  }
}

onMounted(() => {
  void loadAll();
});

function rowKey(row: PolicyView): number {
  return row.id;
}

function storageName(id: number): string {
  return storages.value.find((item) => item.id === id)?.name ?? `ID ${id}`;
}

const WEBP_MODE_META: Record<WebPMode, { label: string; tag: "info" | "success" | "default" }> = {
  both: { label: "原图+WebP", tag: "info" },
  webp_only: { label: "仅 WebP", tag: "success" },
  none: { label: "关闭", tag: "default" },
};

const SCRUB_MODE_LABEL: Record<ScrubMode, string> = {
  none: "不脱敏",
  gps: "抹除 GPS",
  all: "全部脱敏",
};

const columns: DataTableColumns<PolicyView> = [
  { title: "名称", key: "name", width: 150, ellipsis: { tooltip: true } },
  {
    title: "存储",
    key: "storage_id",
    width: 130,
    render: (row) => storageName(row.storage_id),
  },
  {
    title: "启用",
    key: "enabled",
    width: 80,
    render: (row) =>
      h(
        NTag,
        { size: "small", bordered: false, type: row.enabled ? "success" : "default" },
        { default: () => (row.enabled ? "启用" : "停用") },
      ),
  },
  {
    title: "路径模板",
    key: "path_tpl",
    width: 170,
    render: (row) => h("code", null, row.path_tpl),
  },
  {
    title: "文件名模板",
    key: "name_tpl",
    width: 150,
    render: (row) => h("code", null, row.name_tpl),
  },
  {
    title: "WebP",
    key: "webp_mode",
    width: 110,
    render: (row) => {
      const meta = WEBP_MODE_META[row.webp_mode];
      return h(NTag, { size: "small", bordered: false, type: meta.tag }, { default: () => meta.label });
    },
  },
  {
    title: "脱敏",
    key: "scrub_mode",
    width: 110,
    render: (row) => SCRUB_MODE_LABEL[row.scrub_mode],
  },
  {
    title: "操作",
    key: "actions",
    width: 140,
    render: (row) =>
      h(NSpace, { size: 4, wrap: false }, () => [
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
            onPositiveClick: () => removePolicy(row),
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
                { default: () => "删除" },
              ),
            default: () => `使用该规则的用户将无法继续上传。确定删除「${row.name}」吗？`,
          },
        ),
      ]),
  },
];

async function removePolicy(row: PolicyView): Promise<void> {
  removingId.value = row.id;
  try {
    await deletePolicy(row.id);
    message.success(`规则「${row.name}」已删除`);
    await loadAll();
  } catch (error) {
    message.error(`删除失败：${apiErrorMessage(error)}`);
  } finally {
    removingId.value = null;
  }
}

/* ---------------- 新建/编辑规则 ---------------- */

const WEBP_MODE_OPTIONS = [
  { label: "both（原图 + WebP 双版本）", value: "both" },
  { label: "webp_only（仅保留 WebP）", value: "webp_only" },
  { label: "none（不生成 WebP）", value: "none" },
];

const SCRUB_MODE_OPTIONS = [
  { label: "none（保留全部 EXIF）", value: "none" },
  { label: "gps（仅抹除 GPS）", value: "gps" },
  { label: "all（抹除 GPS/序列号/作者/XMP）", value: "all" },
];

const HEIF_MODE_OPTIONS = [
  { label: "webp_only（转 WebP，不保留原图）", value: "webp_only" },
  { label: "keep（保留 HEIC 原图）", value: "keep" },
  { label: "reject（拒绝 HEIC 上传）", value: "reject" },
];

const LINK_PREFER_OPTIONS = [
  { label: "webp（优先 WebP 直链）", value: "webp" },
  { label: "original（优先原图直链）", value: "original" },
];

const ON_CONFLICT_OPTIONS = [
  { label: "rename（自动重命名）", value: "rename" },
  { label: "reject（报错拒绝）", value: "reject" },
];

interface PolicyFormModel {
  name: string;
  storage_id: number | null;
  enabled: boolean;
  path_tpl: string;
  name_tpl: string;
  on_conflict: OnConflict;
  link_prefer: LinkPrefer;
  webp_mode: WebPMode;
  webp_quality: number;
  webp_lossless: boolean;
  webp_effort: number;
  skip_if_larger: boolean;
  max_width: number;
  max_height: number;
  heif_mode: HEIFMode;
  thumb_enabled: boolean;
  thumb_size: number;
  scrub_mode: ScrubMode;
  strip_meta: boolean;
}

function emptyForm(): PolicyFormModel {
  return {
    name: "",
    storage_id: null,
    enabled: true,
    path_tpl: "{Y}/{m}/{d}",
    name_tpl: "{uniqid}",
    on_conflict: "rename",
    link_prefer: "webp",
    webp_mode: "both",
    webp_quality: 80,
    webp_lossless: false,
    webp_effort: 4,
    skip_if_larger: true,
    max_width: 0,
    max_height: 0,
    heif_mode: "webp_only",
    thumb_enabled: true,
    thumb_size: 400,
    scrub_mode: "gps",
    strip_meta: true,
  };
}

const showForm = ref(false);
const formMode = ref<"create" | "edit">("create");
const editing = ref<PolicyView | null>(null);
const submitting = ref(false);
const formRef = ref<FormInst | null>(null);

const form = reactive<PolicyFormModel>(emptyForm());

const storageOptions = computed(() =>
  storages.value.map((item) => ({
    label: item.enabled ? item.name : `${item.name}（已停用）`,
    value: item.id,
  })),
);

function openCreate(): void {
  Object.assign(form, emptyForm());
  formMode.value = "create";
  editing.value = null;
  showForm.value = true;
}

function openEdit(row: PolicyView): void {
  Object.assign(form, emptyForm(), {
    name: row.name,
    storage_id: row.storage_id,
    enabled: row.enabled,
    path_tpl: row.path_tpl,
    name_tpl: row.name_tpl,
    on_conflict: row.on_conflict,
    link_prefer: row.link_prefer,
    webp_mode: row.webp_mode,
    webp_quality: row.webp_quality,
    webp_lossless: row.webp_lossless,
    webp_effort: row.webp_effort,
    skip_if_larger: row.skip_if_larger,
    max_width: row.max_width,
    max_height: row.max_height,
    heif_mode: row.heif_mode,
    thumb_enabled: row.thumb_enabled,
    thumb_size: row.thumb_size,
    scrub_mode: row.scrub_mode,
    strip_meta: row.strip_meta,
  });
  formMode.value = "edit";
  editing.value = row;
  showForm.value = true;
}

const rules: FormRules = {
  name: [
    {
      required: true,
      trigger: ["blur", "input"],
      validator: (_rule: FormItemRule, value: string): boolean | Error =>
        typeof value === "string" && value.trim().length > 0 ? true : new Error("请输入规则名称"),
    },
  ],
  storage_id: [
    {
      required: true,
      trigger: ["blur", "change"],
      validator: (_rule: FormItemRule, value: number | null): boolean | Error =>
        typeof value === "number" ? true : new Error("请选择存储"),
    },
  ],
};

function buildInput(): PolicyInput {
  return {
    name: form.name.trim(),
    storage_id: form.storage_id as number,
    enabled: form.enabled,
    path_tpl: form.path_tpl.trim(),
    name_tpl: form.name_tpl.trim(),
    webp_mode: form.webp_mode,
    webp_quality: form.webp_quality,
    webp_lossless: form.webp_lossless,
    webp_effort: form.webp_effort,
    skip_if_larger: form.skip_if_larger,
    max_width: form.max_width,
    max_height: form.max_height,
    heif_mode: form.heif_mode,
    link_prefer: form.link_prefer,
    on_conflict: form.on_conflict,
    thumb_enabled: form.thumb_enabled,
    thumb_size: form.thumb_size,
    scrub_mode: form.scrub_mode,
    strip_meta: form.strip_meta,
  };
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
      const created = await createPolicy(buildInput());
      message.success(`规则「${created.name}」已创建`);
    } else if (editing.value) {
      const updated = await updatePolicy(editing.value.id, buildInput());
      message.success(`规则「${updated.name}」已保存`);
    }
    showForm.value = false;
    await loadAll();
  } catch (error) {
    const prefix = isCreate ? "创建规则失败" : "保存规则失败";
    message.error(`${prefix}：${apiErrorMessage(error)}`);
  } finally {
    submitting.value = false;
  }
}

/* ---------------- 路径预览 ---------------- */

const previewLoading = ref(false);
const previewResult = ref<PolicyPreviewResult | null>(null);

async function runPreview(): Promise<void> {
  previewLoading.value = true;
  previewResult.value = null;
  try {
    previewResult.value = await previewPolicy({
      path_tpl: form.path_tpl.trim(),
      name_tpl: form.name_tpl.trim(),
    });
  } catch (error) {
    message.error(`路径预览失败：${apiErrorMessage(error)}`);
  } finally {
    previewLoading.value = false;
  }
}
</script>

<template>
  <section class="policies-view">
    <NCard title="规则管理">
      <template #header-extra>
        <NButton type="primary" size="small" @click="openCreate">新建规则</NButton>
      </template>
      <NAlert v-if="loadError" type="error" class="policies-load-error">
        {{ loadError }}
        <NButton size="tiny" quaternary @click="loadAll">重试</NButton>
      </NAlert>
      <NDataTable
        :columns="columns"
        :data="policies"
        :loading="loading"
        :row-key="rowKey"
        :scroll-x="960"
        size="small"
      >
        <template #empty>
          <NEmpty description="暂无规则，点击右上角「新建规则」" />
        </template>
      </NDataTable>
    </NCard>

    <NModal
      v-model:show="showForm"
      preset="card"
      class="policies-form-modal"
      :title="formMode === 'create' ? '新建规则' : '编辑规则'"
    >
      <NForm ref="formRef" :model="form" :rules="rules" label-placement="left" label-width="130">
        <NTabs type="line" default-value="basic" display-directive="show:lazy">
          <NTabPane name="basic" tab="基础">
            <NFormItem label="规则名称" path="name">
              <NInput
                v-model:value="form.name"
                placeholder="例如：默认规则"
                :disabled="submitting"
              />
            </NFormItem>
            <NFormItem label="存储" path="storage_id">
              <NSelect
                v-model:value="form.storage_id"
                :options="storageOptions"
                placeholder="选择存储"
                :disabled="submitting"
              />
            </NFormItem>
            <NFormItem label="启用" path="enabled">
              <NSwitch v-model:value="form.enabled" :disabled="submitting" />
            </NFormItem>
            <NFormItem label="路径模板" path="path_tpl">
              <NInput
                v-model:value="form.path_tpl"
                placeholder="{Y}/{m}/{d}"
                :disabled="submitting"
              />
            </NFormItem>
            <NFormItem label="文件名模板" path="name_tpl">
              <NInput
                v-model:value="form.name_tpl"
                placeholder="{uniqid}"
                :disabled="submitting"
              />
            </NFormItem>
            <NFormItem label="同名冲突" path="on_conflict">
              <NSelect
                v-model:value="form.on_conflict"
                :options="ON_CONFLICT_OPTIONS"
                :disabled="submitting"
              />
            </NFormItem>
            <NFormItem label="直链偏好" path="link_prefer">
              <NSelect
                v-model:value="form.link_prefer"
                :options="LINK_PREFER_OPTIONS"
                :disabled="submitting"
              />
            </NFormItem>
            <div class="policies-preview">
              <NButton size="small" :loading="previewLoading" :disabled="submitting" @click="runPreview">
                预览路径
              </NButton>
              <span v-if="previewResult && !previewResult.error" class="policies-preview__sample">
                样例：{{ previewResult.sample }}
              </span>
              <span v-else-if="previewResult" class="policies-preview__error">
                模板错误：{{ previewResult.error }}
              </span>
            </div>
          </NTabPane>
          <NTabPane name="transcode" tab="转码">
            <NFormItem label="WebP 模式" path="webp_mode">
              <NSelect
                v-model:value="form.webp_mode"
                :options="WEBP_MODE_OPTIONS"
                :disabled="submitting"
              />
            </NFormItem>
            <NFormItem label="WebP 质量" path="webp_quality">
              <NInputNumber
                v-model:value="form.webp_quality"
                :min="1"
                :max="100"
                :disabled="submitting"
              />
            </NFormItem>
            <NFormItem label="WebP 无损" path="webp_lossless">
              <NSwitch v-model:value="form.webp_lossless" :disabled="submitting" />
            </NFormItem>
            <NFormItem label="编码努力" path="webp_effort">
              <NInputNumber
                v-model:value="form.webp_effort"
                :min="0"
                :max="6"
                :disabled="submitting"
              />
            </NFormItem>
            <NFormItem label="更大则跳过" path="skip_if_larger">
              <NSwitch v-model:value="form.skip_if_larger" :disabled="submitting" />
            </NFormItem>
            <NFormItem label="最大宽度" path="max_width">
              <NInputNumber
                v-model:value="form.max_width"
                :min="0"
                :disabled="submitting"
              />
            </NFormItem>
            <NFormItem label="最大高度" path="max_height">
              <NInputNumber
                v-model:value="form.max_height"
                :min="0"
                :disabled="submitting"
              />
            </NFormItem>
            <p class="policies-hint">宽高 0 表示不限制。</p>
            <NFormItem label="HEIC 处理" path="heif_mode">
              <NSelect
                v-model:value="form.heif_mode"
                :options="HEIF_MODE_OPTIONS"
                :disabled="submitting"
              />
            </NFormItem>
          </NTabPane>
          <NTabPane name="thumb" tab="缩略图">
            <NFormItem label="生成缩略图" path="thumb_enabled">
              <NSwitch v-model:value="form.thumb_enabled" :disabled="submitting" />
            </NFormItem>
            <NFormItem label="缩略图边长" path="thumb_size">
              <NInputNumber
                v-model:value="form.thumb_size"
                :min="16"
                :max="4096"
                :disabled="submitting"
              />
            </NFormItem>
          </NTabPane>
          <NTabPane name="privacy" tab="隐私">
            <NFormItem label="EXIF 脱敏" path="scrub_mode">
              <NSelect
                v-model:value="form.scrub_mode"
                :options="SCRUB_MODE_OPTIONS"
                :disabled="submitting"
              />
            </NFormItem>
            <NFormItem label="剥离元数据" path="strip_meta">
              <NSwitch v-model:value="form.strip_meta" :disabled="submitting" />
            </NFormItem>
            <p class="policies-hint">
              脱敏作用于云端副本（无损抹除 GPS/序列号/作者/XMP）；本地库仍保存完整 EXIF。
            </p>
          </NTabPane>
        </NTabs>
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
  </section>
</template>

<style scoped>
.policies-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.policies-load-error {
  margin-bottom: 12px;
}

.policies-form-modal {
  width: 640px;
}

.policies-preview {
  display: flex;
  align-items: center;
  gap: 12px;
  margin: 4px 0 0 130px;
  font-size: 13px;
  word-break: break-all;
}

.policies-preview__sample {
  color: #18a058;
}

.policies-preview__error {
  color: #d03050;
}

.policies-hint {
  margin: 0 0 16px 130px;
  font-size: 13px;
  color: rgba(128, 128, 128, 0.9);
}
</style>
