<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { Page } from "@vben/common-ui";
import { NAlert, NButton, NCard, NSelect, NSpin, NSwitch, NTag, type SelectOption } from "naive-ui";
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import UploadDropzone from "@/components/upload/UploadDropzone.vue";
import UploadQueueList from "@/components/upload/UploadQueueList.vue";
import { useUploadQueue } from "@/components/upload/useUploadQueue";
import { listPolicies } from "@/api/policies";
import { listAlbums, type AlbumView } from "@/api/albums";
import type { PolicySummary } from "@/api/types";
import { MAX_UPLOAD_FILES, MAX_UPLOAD_FILE_BYTES, MAX_UPLOAD_TOTAL_BYTES } from "@/api/upload";
import { formatBytes } from "@/lib/format";
import { formatApiError } from "@/locales/errors";

const { t } = useI18n();
const policies = ref<PolicySummary[] | null>(null);
const policiesError = ref<unknown>(null);
const policiesLoading = ref(false);
const policyId = ref<number | null>(null);
const isPublic = ref(false);
const albums = ref<AlbumView[]>([]);
const albumsFailed = ref(false);
const albumsLoading = ref(false);
const albumId = ref<number | null>(null);
let active = true;
onBeforeUnmount(() => {
  active = false;
});
const policyOptions = computed<SelectOption[]>(() =>
  (policies.value ?? []).map((policy) => ({ label: policy.name, value: policy.id })),
);
const albumOptions = computed<SelectOption[]>(() =>
  albums.value.map((album) => ({ label: album.name, value: album.id })),
);
const {
  items,
  uploading,
  canUpload,
  preflightErrors,
  clearPreflight,
  onFiles,
  startUpload,
  retryItem,
} = useUploadQueue(() => ({
  policyId: policyId.value,
  albumId: albumId.value,
  isPublic: isPublic.value,
}));
onMounted(() => {
  void loadPolicies();
  void loadAlbums();
});
async function loadPolicies() {
  if (policiesLoading.value) return;
  policiesLoading.value = true;
  policiesError.value = null;
  try {
    const list = await listPolicies();
    if (!active) return;
    policies.value = list;
    policyId.value = list.length === 1 ? list[0]!.id : null;
  } catch (error) {
    if (active) policiesError.value = error;
  } finally {
    if (active) policiesLoading.value = false;
  }
}
async function loadAlbums() {
  if (albumsLoading.value) return;
  albumsLoading.value = true;
  albumsFailed.value = false;
  try {
    const data = await listAlbums({ page: 1, size: 100 });
    if (active) albums.value = data.items;
  } catch {
    if (active) albumsFailed.value = true;
  } finally {
    if (active) albumsLoading.value = false;
  }
}
</script>

<template>
  <Page
    class="upload-view"
    :description="t('user.upload.description')"
    header-class="flex-wrap gap-4"
  >
    <template #title
      ><h1 class="upload-view__title">{{ t("user.upload.title") }}</h1></template
    >

    <div class="upload-view__content">
      <NAlert v-if="preflightErrors.length > 0" type="error" closable @close="clearPreflight">
        <div v-for="error in preflightErrors" :key="error">{{ error }}</div>
      </NAlert>

      <NCard :title="t('user.upload.choose')" :bordered="false">
        <UploadDropzone :disabled="uploading" @files="onFiles" />
        <p class="upload-view__limits">
          {{
            t("user.upload.limits", {
              count: MAX_UPLOAD_FILES,
              fileSize: formatBytes(MAX_UPLOAD_FILE_BYTES),
              totalSize: formatBytes(MAX_UPLOAD_TOTAL_BYTES),
            })
          }}
        </p>

        <div class="upload-view__options">
          <div class="upload-view__policy-field">
            <span class="upload-view__label">{{ t("user.upload.rule") }}</span>
            <div v-if="policiesError" class="upload-view__empty-policy">
              {{ formatApiError(policiesError, "user.upload.rulesError") }}
              <NButton text size="small" :disabled="policiesLoading" @click="loadPolicies">{{
                t("user.common.retry")
              }}</NButton>
            </div>
            <NSpin v-else-if="policies === null" size="small" />
            <span v-else-if="policies.length === 0" class="upload-view__empty-policy">
              {{ t("user.upload.noRules") }}
            </span>
            <NSelect
              v-else
              v-model:value="policyId"
              class="upload-view__policy"
              :options="policyOptions"
              :placeholder="t('user.upload.chooseRule')"
              :aria-label="t('user.upload.rule')"
              :disabled="uploading"
            />
          </div>
          <div class="upload-view__album-field">
            <span class="upload-view__label">{{ t("user.upload.album") }}</span>
            <NSelect
              v-model:value="albumId"
              class="upload-view__album"
              :options="albumOptions"
              :placeholder="t('user.upload.noAlbum')"
              :aria-label="t('user.upload.album')"
              :disabled="uploading || albumsLoading"
              :loading="albumsLoading"
              clearable
            />
            <div v-if="albumsFailed" class="upload-view__empty-policy">
              {{ t("user.upload.albumsError") }}
              <NButton text size="small" :disabled="uploading" @click="loadAlbums">{{
                t("user.common.retry")
              }}</NButton>
            </div>
          </div>
          <div class="upload-view__visibility-field">
            <span class="upload-view__label">{{ t("user.upload.visibility") }}</span>
            <NSwitch
              v-model:value="isPublic"
              :aria-label="t('user.upload.publicLabel')"
              :disabled="uploading"
            >
              <template #checked>{{ t("user.common.public") }}</template>
              <template #unchecked>{{ t("user.common.private") }}</template>
            </NSwitch>
          </div>
          <NButton
            class="upload-view__submit"
            type="primary"
            :loading="uploading"
            :disabled="!canUpload"
            @click="startUpload"
          >
            {{ t("user.upload.start") }}
          </NButton>
        </div>
        <p class="upload-view__privacy">{{ t("user.upload.privacy") }}</p>
      </NCard>

      <NCard :title="t('user.upload.queue')" :bordered="false">
        <template #header-extra>
          <NTag :bordered="false" size="small" :aria-label="t('user.upload.queueCount')">
            {{ t("user.upload.files", { count: items.length }) }}
          </NTag>
        </template>
        <UploadQueueList :items="items" :busy="uploading" @retry="retryItem" />
      </NCard>
    </div>
  </Page>
</template>

<style scoped>
.upload-view__title {
  margin: 0 0 8px;
  font-size: 18px;
  font-weight: 600;
}

.upload-view__content {
  display: grid;
  gap: 16px;
  min-width: 0;
}

.upload-view__limits,
.upload-view__privacy {
  margin: 12px 0 0;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
  line-height: 1.7;
}

.upload-view__options {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 20px;
  margin-top: 24px;
}

.upload-view__album-field,
.upload-view__policy-field {
  flex: 1 1 280px;
  min-width: 0;
}

.upload-view__label {
  display: block;
  margin-bottom: 8px;
  font-size: 13px;
}

.upload-view__visibility-field {
  padding-bottom: 6px;
}

.upload-view__empty-policy {
  font-size: 13px;
  color: hsl(var(--muted-foreground));
}

@media (max-width: 640px) {
  .upload-view__options {
    gap: 16px;
  }

  .upload-view__album-field,
  .upload-view__policy-field {
    flex-basis: 100%;
  }

  .upload-view__submit {
    margin-left: auto;
  }
}
</style>
