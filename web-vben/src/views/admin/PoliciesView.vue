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
  NTabPane,
  NTabs,
  NTag,
  useMessage,
  type DataTableColumns,
  type FormInst,
  type FormItemRule,
  type FormRules,
} from "naive-ui";
import { computed, h, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
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

const { t } = useI18n();
const message = useMessage();
const isMobile = useMediaQuery("(max-width: 640px)");

/* ---------------- 列表 ---------------- */

const policies = ref<PolicyView[]>([]);
const storages = ref<StorageView[]>([]);
const loading = ref(false);
const loadError = ref<unknown>(null);
const removingId = ref<number | null>(null);

async function loadAll(): Promise<void> {
  loading.value = true;
  loadError.value = null;
  try {
    const [policyList, storageList] = await Promise.all([listPolicies(), listStorages()]);
    policies.value = policyList;
    storages.value = storageList;
  } catch (error) {
    loadError.value = error;
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

const WEBP_MODE_META = computed<
  Record<WebPMode, { label: string; tag: "info" | "success" | "default" }>
>(() => ({
  both: { label: t("admin.policies.both"), tag: "info" },
  webp_only: { label: t("admin.policies.webpOnly"), tag: "success" },
  none: { label: t("admin.policies.off"), tag: "default" },
}));

const SCRUB_MODE_LABEL = computed<Record<ScrubMode, string>>(() => ({
  none: t("admin.policies.scrubNone"),
  gps: t("admin.policies.scrubGps"),
  all: t("admin.policies.scrubAll"),
}));

const columns = computed<DataTableColumns<PolicyView>>(() => [
  { title: t("admin.common.name"), key: "name", width: 150, ellipsis: { tooltip: true } },
  {
    title: t("admin.common.storage"),
    key: "storage_id",
    width: 130,
    render: (row) => storageName(row.storage_id),
  },
  {
    title: t("admin.common.enabled"),
    key: "enabled",
    width: 80,
    render: (row) =>
      h(
        NTag,
        { size: "small", bordered: false, type: row.enabled ? "success" : "default" },
        { default: () => (row.enabled ? t("admin.common.enabled") : t("admin.common.disabled")) },
      ),
  },
  {
    title: t("admin.policies.pathTemplate"),
    key: "path_tpl",
    width: 170,
    render: (row) => h("code", null, row.path_tpl),
  },
  {
    title: t("admin.policies.nameTemplate"),
    key: "name_tpl",
    width: 150,
    render: (row) => h("code", null, row.name_tpl),
  },
  {
    title: "WebP",
    key: "webp_mode",
    width: 110,
    render: (row) => {
      const meta = WEBP_MODE_META.value[row.webp_mode];
      return h(
        NTag,
        { size: "small", bordered: false, type: meta.tag },
        { default: () => meta.label },
      );
    },
  },
  {
    title: t("admin.policies.scrub"),
    key: "scrub_mode",
    width: 110,
    render: (row) => SCRUB_MODE_LABEL.value[row.scrub_mode],
  },
  {
    title: t("admin.common.actions"),
    key: "actions",
    width: 140,
    render: (row) =>
      h(NSpace, { size: 4, wrap: false }, () => [
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
                { default: () => t("admin.common.delete") },
              ),
            default: () => t("admin.policies.deleteConfirm", { name: row.name }),
          },
        ),
      ]),
  },
]);

async function removePolicy(row: PolicyView): Promise<void> {
  removingId.value = row.id;
  try {
    await deletePolicy(row.id);
    message.success(() => t("admin.policies.deleted", { name: row.name }));
    await loadAll();
  } catch (error) {
    message.error(() => formatApiError(error, "admin.errors.delete"));
  } finally {
    removingId.value = null;
  }
}

/* ---------------- 新建/编辑规则 ---------------- */

const WEBP_MODE_OPTIONS = computed(() => [
  { label: t("admin.policies.webpBothOption"), value: "both" },
  { label: t("admin.policies.webpOnlyOption"), value: "webp_only" },
  { label: t("admin.policies.webpNoneOption"), value: "none" },
]);

const SCRUB_MODE_OPTIONS = computed(() => [
  { label: t("admin.policies.scrubNoneOption"), value: "none" },
  { label: t("admin.policies.scrubGpsOption"), value: "gps" },
  { label: t("admin.policies.scrubAllOption"), value: "all" },
]);

const HEIF_MODE_OPTIONS = computed(() => [
  { label: t("admin.policies.heifWebpOption"), value: "webp_only" },
  { label: t("admin.policies.heifKeepOption"), value: "keep" },
  { label: t("admin.policies.heifRejectOption"), value: "reject" },
]);

const LINK_PREFER_OPTIONS = computed(() => [
  { label: t("admin.policies.linkWebpOption"), value: "webp" },
  { label: t("admin.policies.linkOriginalOption"), value: "original" },
]);

const ON_CONFLICT_OPTIONS = computed(() => [
  { label: t("admin.policies.renameOption"), value: "rename" },
  { label: t("admin.policies.rejectOption"), value: "reject" },
]);

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
    label: item.enabled ? item.name : t("admin.policies.disabledStorage", { name: item.name }),
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

const rules = computed<FormRules>(() => ({
  name: [
    {
      required: true,
      trigger: ["blur", "input"],
      renderMessage: () => t("admin.policies.nameRequired"),
      validator: (_rule: FormItemRule, value: string): boolean | Error =>
        typeof value === "string" && value.trim().length > 0
          ? true
          : new Error(t("admin.policies.nameRequired")),
    },
  ],
  storage_id: [
    {
      required: true,
      trigger: ["blur", "change"],
      renderMessage: () => t("admin.policies.storageRequired"),
      validator: (_rule: FormItemRule, value: number | null): boolean | Error =>
        typeof value === "number" ? true : new Error(t("admin.policies.storageRequired")),
    },
  ],
}));

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
      const created = await createPolicy(buildInput());
      message.success(() => t("admin.policies.created", { name: created.name }));
    } else if (editing.value) {
      const updated = await updatePolicy(editing.value.id, buildInput());
      message.success(() => t("admin.policies.saved", { name: updated.name }));
    }
    showForm.value = false;
    await loadAll();
  } catch (error) {
    message.error(() =>
      formatApiError(error, isCreate ? "admin.errors.policyCreate" : "admin.errors.policySave"),
    );
  } finally {
    submitting.value = false;
  }
}

/* ---------------- 路径预览 ---------------- */

const previewLoading = ref(false);
const previewResult = ref<PolicyPreviewResult | null>(null);
const previewError = computed(() =>
  formatApiError(previewResult.value?.error, "admin.errors.templateInvalid"),
);
let previewRequest = 0;

/** 模板或编辑会话变化时，旧结果和迟到响应都不能再代表当前表单。 */
function resetPreview(): void {
  previewRequest += 1;
  previewResult.value = null;
  previewLoading.value = false;
}

watch([() => form.path_tpl, () => form.name_tpl, showForm], resetPreview, { flush: "sync" });
onBeforeUnmount(resetPreview);

async function runPreview(): Promise<void> {
  if (previewLoading.value || submitting.value || !showForm.value) {
    return;
  }
  const request = ++previewRequest;
  previewLoading.value = true;
  previewResult.value = null;
  try {
    const result = await previewPolicy({
      path_tpl: form.path_tpl.trim(),
      name_tpl: form.name_tpl.trim(),
    });
    if (request === previewRequest) {
      previewResult.value = result;
    }
  } catch (error) {
    if (request === previewRequest) {
      message.error(() => formatApiError(error, "admin.errors.preview"));
    }
  } finally {
    if (request === previewRequest) {
      previewLoading.value = false;
    }
  }
}
</script>

<template>
  <Page
    :title="t('admin.policies.title')"
    :description="t('admin.policies.description')"
    class="policies-view min-w-0"
    header-class="flex-wrap items-center gap-4"
    content-class="min-w-0"
  >
    <template #extra>
      <NButton type="primary" size="small" @click="openCreate">{{
        t("admin.policies.create")
      }}</NButton>
    </template>
    <NCard :bordered="false" class="min-w-0">
      <NAlert v-if="loadError" type="error" class="policies-load-error">
        {{ formatApiError(loadError, "admin.errors.rulesLoad") }}
        <NButton size="tiny" quaternary @click="loadAll">{{ t("admin.common.retry") }}</NButton>
      </NAlert>
      <NDataTable
        :columns="columns"
        :data="policies"
        :loading="loading"
        :row-key="rowKey"
        :scroll-x="1040"
        size="small"
      >
        <template #empty>
          <NEmpty :description="t('admin.policies.empty')" />
        </template>
      </NDataTable>
    </NCard>

    <NModal
      v-model:show="showForm"
      preset="card"
      :mask-closable="!submitting"
      :close-on-esc="!submitting"
      :closable="!submitting"
      class="policies-form-modal"
      :title="formMode === 'create' ? t('admin.policies.create') : t('admin.policies.edit')"
    >
      <NForm
        ref="formRef"
        :model="form"
        :rules="rules"
        :label-placement="isMobile ? 'top' : 'left'"
        :label-width="isMobile ? undefined : 130"
      >
        <NTabs type="line" default-value="basic" display-directive="show:lazy">
          <NTabPane name="basic" :tab="t('admin.policies.basic')">
            <NFormItem :label="t('admin.policies.name')" path="name">
              <NInput
                v-model:value="form.name"
                :placeholder="t('admin.policies.namePlaceholder')"
                :disabled="submitting"
              />
            </NFormItem>
            <NFormItem :label="t('admin.common.storage')" path="storage_id">
              <NSelect
                v-model:value="form.storage_id"
                :options="storageOptions"
                :placeholder="t('admin.policies.selectStorage')"
                :disabled="submitting"
              />
            </NFormItem>
            <NFormItem :label="t('admin.common.enabled')" path="enabled">
              <NSwitch v-model:value="form.enabled" :disabled="submitting" />
            </NFormItem>
            <NFormItem :label="t('admin.policies.pathTemplate')" path="path_tpl">
              <NInput
                v-model:value="form.path_tpl"
                placeholder="{Y}/{m}/{d}"
                :disabled="submitting"
              />
            </NFormItem>
            <NFormItem :label="t('admin.policies.nameTemplate')" path="name_tpl">
              <NInput v-model:value="form.name_tpl" placeholder="{uniqid}" :disabled="submitting" />
            </NFormItem>
            <NFormItem :label="t('admin.policies.conflict')" path="on_conflict">
              <NSelect
                v-model:value="form.on_conflict"
                :options="ON_CONFLICT_OPTIONS"
                :disabled="submitting"
              />
            </NFormItem>
            <NFormItem :label="t('admin.policies.linkPrefer')" path="link_prefer">
              <NSelect
                v-model:value="form.link_prefer"
                :options="LINK_PREFER_OPTIONS"
                :disabled="submitting"
              />
            </NFormItem>
            <div class="policies-preview">
              <NButton
                size="small"
                :loading="previewLoading"
                :disabled="submitting"
                @click="runPreview"
              >
                {{ t("admin.policies.preview") }}
              </NButton>
              <span v-if="previewResult && !previewResult.error" class="policies-preview__sample">
                {{ t("admin.policies.sample", { sample: previewResult.sample }) }}
              </span>
              <span v-else-if="previewResult" class="policies-preview__error">
                {{ t("admin.policies.previewInvalid", { error: previewError }) }}
              </span>
            </div>
          </NTabPane>
          <NTabPane name="transcode" :tab="t('admin.policies.transcode')">
            <NFormItem :label="t('admin.policies.webpMode')" path="webp_mode">
              <NSelect
                v-model:value="form.webp_mode"
                :options="WEBP_MODE_OPTIONS"
                :disabled="submitting"
              />
            </NFormItem>
            <NFormItem :label="t('admin.policies.webpQuality')" path="webp_quality">
              <NInputNumber
                v-model:value="form.webp_quality"
                :min="1"
                :max="100"
                :disabled="submitting"
              />
            </NFormItem>
            <NFormItem :label="t('admin.policies.webpLossless')" path="webp_lossless">
              <NSwitch v-model:value="form.webp_lossless" :disabled="submitting" />
            </NFormItem>
            <NFormItem :label="t('admin.policies.effort')" path="webp_effort">
              <NInputNumber
                v-model:value="form.webp_effort"
                :min="0"
                :max="6"
                :disabled="submitting"
              />
            </NFormItem>
            <NFormItem :label="t('admin.policies.skipLarger')" path="skip_if_larger">
              <NSwitch v-model:value="form.skip_if_larger" :disabled="submitting" />
            </NFormItem>
            <NFormItem :label="t('admin.policies.maxWidth')" path="max_width">
              <NInputNumber v-model:value="form.max_width" :min="0" :disabled="submitting" />
            </NFormItem>
            <NFormItem :label="t('admin.policies.maxHeight')" path="max_height">
              <NInputNumber v-model:value="form.max_height" :min="0" :disabled="submitting" />
            </NFormItem>
            <p class="policies-hint">{{ t("admin.policies.dimensionsHint") }}</p>
            <NFormItem :label="t('admin.policies.heif')" path="heif_mode">
              <NSelect
                v-model:value="form.heif_mode"
                :options="HEIF_MODE_OPTIONS"
                :disabled="submitting"
              />
            </NFormItem>
          </NTabPane>
          <NTabPane name="thumb" :tab="t('admin.policies.thumbnail')">
            <NFormItem :label="t('admin.policies.thumbEnabled')" path="thumb_enabled">
              <NSwitch v-model:value="form.thumb_enabled" :disabled="submitting" />
            </NFormItem>
            <NFormItem :label="t('admin.policies.thumbSize')" path="thumb_size">
              <NInputNumber
                v-model:value="form.thumb_size"
                :min="16"
                :max="4096"
                :disabled="submitting"
              />
            </NFormItem>
          </NTabPane>
          <NTabPane name="privacy" :tab="t('admin.policies.privacy')">
            <NFormItem :label="t('admin.policies.scrubMode')" path="scrub_mode">
              <NSelect
                v-model:value="form.scrub_mode"
                :options="SCRUB_MODE_OPTIONS"
                :disabled="submitting"
              />
            </NFormItem>
            <NFormItem :label="t('admin.policies.stripMeta')" path="strip_meta">
              <NSwitch v-model:value="form.strip_meta" :disabled="submitting" />
            </NFormItem>
            <p class="policies-hint">
              {{ t("admin.policies.privacyHint") }}
            </p>
          </NTabPane>
        </NTabs>
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
  </Page>
</template>

<style scoped>
.policies-load-error {
  margin-bottom: 12px;
}

.policies-form-modal {
  width: 640px;
  max-width: calc(100vw - 32px);
}

.policies-preview {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  margin: 4px 0 0 130px;
  font-size: 13px;
  word-break: break-all;
}

.policies-preview__sample {
  color: hsl(var(--success));
}

.policies-preview__error {
  color: hsl(var(--destructive));
}

.policies-hint {
  margin: 0 0 16px 130px;
  font-size: 13px;
  color: hsl(var(--muted-foreground));
}
@media (max-width: 640px) {
  .policies-preview,
  .policies-hint {
    margin-left: 0;
  }
}
</style>
