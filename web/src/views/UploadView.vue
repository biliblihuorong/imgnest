<script setup lang="ts">
import { NAlert, NButton, NSelect, NSpin, NSwitch, type SelectOption } from "naive-ui";
import { computed, onMounted, ref } from "vue";
import UploadDropzone from "@/components/upload/UploadDropzone.vue";
import UploadQueueList from "@/components/upload/UploadQueueList.vue";
import type { UploadQueueItem } from "@/components/upload/types";
import { listPolicies } from "@/api/policies";
import type { PolicySummary } from "@/api/types";
import {
  MAX_UPLOAD_FILES,
  MAX_UPLOAD_FILE_BYTES,
  MAX_UPLOAD_TOTAL_BYTES,
  preflightUpload,
  uploadImages,
  type UploadItemResult,
} from "@/api/upload";
import { formatBytes } from "@/lib/format";

const POLICY_EMPTY_GUIDE = "当前用户组没有可用的上传规则，请联系管理员";

const policies = ref<PolicySummary[] | null>(null);
const policiesError = ref<string | null>(null);
const policyId = ref<number | null>(null);
const isPublic = ref(false);

const items = ref<UploadQueueItem[]>([]);
let nextItemId = 1;

const preflightErrors = ref<string[]>([]);
const uploading = ref(false);

const policyOptions = computed<SelectOption[]>(() =>
  (policies.value ?? []).map((policy) => ({ label: policy.name, value: policy.id })),
);

const canUpload = computed(
  () => policyId.value !== null && !uploading.value && items.value.some((item) => item.state === "queued"),
);

onMounted(() => {
  void loadPolicies();
});

async function loadPolicies(): Promise<void> {
  policiesError.value = null;
  try {
    const list = await listPolicies();
    policies.value = list;
    // 恰好一条时自动选中
    if (list.length === 1) {
      policyId.value = list[0].id;
    }
  } catch (error) {
    policiesError.value = error instanceof Error ? error.message : "上传规则加载失败";
  }
}

function onFiles(files: File[]): void {
  const result = preflightUpload(files);
  preflightErrors.value = result.errors;
  if (!result.ok) {
    return;
  }
  for (const file of files) {
    items.value.push({
      id: nextItemId++,
      file,
      name: file.name,
      size: file.size,
      state: "queued",
      loaded: 0,
      total: file.size,
      error: null,
      image: null,
    });
  }
}

function startUpload(): void {
  const targets = items.value.filter((item) => item.state === "queued");
  if (policyId.value === null || targets.length === 0 || uploading.value) {
    return;
  }
  void runUpload(targets);
}

async function retryItem(item: UploadQueueItem): Promise<void> {
  if (uploading.value || item.state !== "failed") {
    return;
  }
  await runUpload([item]);
}

async function runUpload(targets: UploadQueueItem[]): Promise<void> {
  uploading.value = true;
  try {
    const results = await uploadImages({
      files: targets.map((item) => item.file),
      policyId: policyId.value,
      isPublic: isPublic.value,
      onFileStart: (index, file) => {
        const item = targets[index];
        if (!item) {
          return;
        }
        item.state = "uploading";
        item.loaded = 0;
        item.total = file.size;
      },
      onFileProgress: (index, _file, loaded, total) => {
        const item = targets[index];
        if (!item) {
          return;
        }
        item.loaded = loaded;
        item.total = total;
      },
      onFileSettled: (index, _file, result) => {
        applyResult(targets[index], result);
      },
    });
    // 兜底：以上传函数的返回结果刷新终态（回调缺失时保证队列最终一致）
    results.forEach((result, index) => applyResult(targets[index], result));
  } finally {
    uploading.value = false;
  }
}

function applyResult(item: UploadQueueItem | undefined, result: UploadItemResult): void {
  if (!item) {
    return;
  }
  if (result.ok) {
    item.state = "success";
    item.image = result.image;
    item.error = null;
  } else {
    item.state = "failed";
    item.error = { code: result.code, message: result.message, status: result.status };
  }
}
</script>

<template>
  <section class="upload-view">
    <h2>上传</h2>

    <NAlert v-if="preflightErrors.length > 0" type="error" closable @close="preflightErrors = []">
      <div v-for="error in preflightErrors" :key="error">{{ error }}</div>
    </NAlert>

    <UploadDropzone :disabled="uploading" @files="onFiles" />
    <p class="upload-view__limits">
      单次最多 {{ MAX_UPLOAD_FILES }} 个文件，单文件不超过
      {{ formatBytes(MAX_UPLOAD_FILE_BYTES) }}，合计不超过
      {{ formatBytes(MAX_UPLOAD_TOTAL_BYTES) }}（与服务端配置一致）。
    </p>

    <div class="upload-view__options">
      <span v-if="policiesError" class="upload-view__empty-policy">{{ policiesError }}</span>
      <NSpin v-else-if="policies === null" size="small" />
      <span v-else-if="policies.length === 0" class="upload-view__empty-policy">
        {{ POLICY_EMPTY_GUIDE }}
      </span>
      <NSelect
        v-else
        v-model:value="policyId"
        class="upload-view__policy"
        :options="policyOptions"
        placeholder="选择上传规则"
        :disabled="uploading"
      />
      <label class="upload-view__public">
        <NSwitch v-model:value="isPublic" :disabled="uploading">
          <template #checked>公开</template>
          <template #unchecked>私有</template>
        </NSwitch>
      </label>
      <NButton type="primary" :loading="uploading" :disabled="!canUpload" @click="startUpload">
        开始上传
      </NButton>
    </div>

    <UploadQueueList :items="items" @retry="retryItem" />
  </section>
</template>

<style scoped>
.upload-view {
  max-width: 860px;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.upload-view h2 {
  margin: 0;
}

.upload-view__limits {
  margin: 0;
  font-size: 12px;
  opacity: 0.7;
}

.upload-view__options {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 16px;
}

.upload-view__policy {
  flex: 1;
  min-width: 200px;
}

.upload-view__empty-policy {
  font-size: 13px;
  opacity: 0.75;
}

.upload-view__public {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
</style>
