<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { Page } from "@vben/common-ui";
import {
  NAlert,
  NButton,
  NCard,
  NRadioButton,
  NRadioGroup,
  NSelect,
  NSpin,
  NSwitch,
  NTag,
  useMessage,
  type SelectOption,
} from "naive-ui";
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useTemplateRef } from "vue";
import type { ImageView } from "@/api/upload";
import ImageCopyDrawer from "@/components/images/ImageCopyDrawer.vue";
import UploadDropzone from "@/components/upload/UploadDropzone.vue";
import UploadQueueList from "@/components/upload/UploadQueueList.vue";
import { useUploadQueue } from "@/components/upload/useUploadQueue";
import { buildLinkText, resolveImageLink, type LinkFormat, type LinkVersion } from "@/components/upload/linkText";
import { listPolicies } from "@/api/policies";
import type { AlbumView } from "@/api/albums";
import { listAllAlbums } from "@/api/allAlbums";
import type { PolicySummary } from "@/api/types";
import { MAX_UPLOAD_FILES, MAX_UPLOAD_FILE_BYTES, MAX_UPLOAD_TOTAL_BYTES } from "@/api/upload";
import { copyText } from "@/lib/clipboard";
import { formatBytes } from "@/lib/format";
import { formatApiError } from "@/locales/errors";

const { t } = useI18n();
const message = useMessage();
const policies = ref<PolicySummary[] | null>(null);
const policiesError = ref<unknown>(null);
const policiesLoading = ref(false);
const policyId = ref<number | null>(null);
const isPublic = ref(false);
const albums = ref<AlbumView[]>([]);
const albumsFailed = ref(false);
const albumsLoading = ref(false);
const albumId = ref<number | null>(null);
// 队列级链接选择：切换一次对全部成功项生效；默认 WebP，缺失时逐项回退 links.url。
const linkFormat = ref<LinkFormat>("url");
const linkVersion = ref<LinkVersion>("webp");
const queueCard = useTemplateRef<InstanceType<typeof NCard>>("queueCard");
const queueFlash = ref(false);
// 复制抽屉：展示某张成功图的完整「版本 × 格式」矩阵。
const drawerShow = ref(false);
const drawerImage = ref<ImageView | null>(null);
let flashTimer: ReturnType<typeof setTimeout> | undefined;
let active = true;
onBeforeUnmount(() => {
  active = false;
  if (flashTimer) clearTimeout(flashTimer);
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

const successItems = computed(() =>
  items.value.filter((item) => item.state === "success" && item.image),
);

/** 入队后立即把队列带到用户眼前，避免在大屏上四处寻找。 */
async function handleFiles(files: File[]): Promise<void> {
  const before = items.value.length;
  onFiles(files);
  if (items.value.length === before) return;
  message.success(() => t("user.upload.added", { count: items.value.length - before }));
  await nextTick();
  const el = (queueCard.value?.$el as HTMLElement | undefined) ?? null;
  el?.scrollIntoView({ behavior: "smooth", block: "start" });
  queueFlash.value = true;
  if (flashTimer) clearTimeout(flashTimer);
  flashTimer = setTimeout(() => {
    queueFlash.value = false;
  }, 1200);
}

function openDrawer(image: ImageView): void {
  drawerImage.value = image;
  drawerShow.value = true;
}

/** 按当前全局格式/版本复制全部成功项链接，每行一条。 */
async function copyAllLinks(): Promise<void> {
  const targets = successItems.value;
  if (!targets.length) return;
  const text = targets
    .map((item) =>
      buildLinkText(
        linkFormat.value,
        item.image!.name || item.image!.key,
        resolveImageLink(item.image!, linkVersion.value),
      ),
    )
    .join("\n");
  const ok = await copyText(text);
  if (active) {
    if (ok) message.success(() => t("user.upload.copiedAll", { count: targets.length }));
    else message.error(t("user.upload.copyError"));
  }
}
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
    const all = await listAllAlbums();
    if (active) albums.value = all;
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

      <NCard :title="t('user.upload.choose')" :bordered="false" :content-style="{ padding: '14px 18px 18px' }">
        <UploadDropzone :disabled="uploading" @files="handleFiles" />
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

      <NCard
        ref="queueCard"
        :title="t('user.upload.queue')"
        :bordered="false"
        :content-style="{ padding: '10px 18px 18px' }"
        :class="{ 'upload-view__queue--flash': queueFlash }"
      >
        <template #header-extra>
          <div class="upload-view__queue-extra">
            <NTag :bordered="false" size="small" :aria-label="t('user.upload.queueCount')">
              {{ t("user.upload.files", { count: items.length }) }}
            </NTag>
            <NButton
              v-if="successItems.length > 0"
              size="small"
              type="primary"
              secondary
              @click="copyAllLinks"
            >
              {{ t("user.upload.copyAll") }}
            </NButton>
          </div>
        </template>
        <div
          v-if="successItems.length > 0"
          class="upload-view__link-controls"
          role="group"
          :aria-label="t('user.upload.linkFormat')"
        >
          <NRadioGroup v-model:value="linkVersion" size="small" :aria-label="t('user.upload.linkVersion')">
            <NRadioButton value="original">{{ t("user.upload.original") }}</NRadioButton>
            <NRadioButton value="webp">{{ t("user.upload.webp") }}</NRadioButton>
            <NRadioButton value="thumbnail">{{ t("user.upload.thumbnail") }}</NRadioButton>
          </NRadioGroup>
          <NRadioGroup v-model:value="linkFormat" size="small" :aria-label="t('user.upload.linkFormat')">
            <NRadioButton value="url">URL</NRadioButton>
            <NRadioButton value="markdown">Markdown</NRadioButton>
            <NRadioButton value="html">HTML</NRadioButton>
            <NRadioButton value="bbcode">BBCode</NRadioButton>
          </NRadioGroup>
        </div>
        <UploadQueueList
          v-model:format="linkFormat"
          v-model:version="linkVersion"
          :items="items"
          :busy="uploading"
          @retry="retryItem"
          @detail="openDrawer"
        />
      </NCard>
    </div>
    <ImageCopyDrawer v-model:show="drawerShow" :image="drawerImage" />
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
  gap: 12px;
  min-width: 0;
}

.upload-view__limits,
.upload-view__privacy {
  margin: 8px 0 0;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
  line-height: 1.7;
}

.upload-view__options {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 12px;
  margin-top: 16px;
}

.upload-view__album-field,
.upload-view__policy-field {
  flex: 1 1 240px;
  min-width: 0;
}

.upload-view__label {
  display: block;
  margin-bottom: 6px;
  font-size: 13px;
}

.upload-view__visibility-field {
  padding-bottom: 6px;
}

.upload-view__empty-policy {
  font-size: 13px;
  color: hsl(var(--muted-foreground));
}

.upload-view__queue-extra {
  display: flex;
  align-items: center;
  gap: 8px;
}

.upload-view__link-controls {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 20px;
  padding: 8px 12px;
  margin-bottom: 4px;
  border-radius: 6px;
  background-color: hsl(var(--muted) / 0.5);
}

.upload-view__link-controls :deep(.n-radio-group) {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 0;
}

/* 入队瞬间的高亮，引导视线落到队列卡片上 */
.upload-view__queue--flash {
  animation: upload-queue-flash 1.2s ease-out;
}

@keyframes upload-queue-flash {
  0% {
    box-shadow: 0 0 0 3px hsl(var(--primary) / 0.45);
  }
  100% {
    box-shadow: 0 0 0 3px transparent;
  }
}

@media (max-width: 640px) {
  .upload-view__options {
    gap: 12px;
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
