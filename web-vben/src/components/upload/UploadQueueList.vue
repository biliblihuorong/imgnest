<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { ApiError } from "@/api/client";
import { formatApiError } from "@/locales/errors";
import { formatBytes } from "@/lib/format";
import { NButton, NEmpty, NProgress, NTag } from "naive-ui";
import UploadResultActions from "./UploadResultActions.vue";
import type { UploadQueueItem, UploadQueueState } from "./types";

defineProps<{ items: UploadQueueItem[]; busy?: boolean }>();
const { t } = useI18n();

const emit = defineEmits<{ retry: [item: UploadQueueItem] }>();

const STATE_TAG_TYPE: Record<UploadQueueState, "default" | "info" | "success" | "error"> = {
  queued: "default",
  uploading: "info",
  success: "success",
  failed: "error",
};

function progressPercent(item: UploadQueueItem): number {
  if (item.total <= 0) {
    return 0;
  }
  return Math.min(100, Math.round((item.loaded / item.total) * 100));
}
</script>

<template>
  <ul class="upload-queue">
    <li v-if="items.length === 0" class="upload-queue__empty">
      <NEmpty :description="t('user.upload.emptyQueue')" />
    </li>
    <li v-for="item in items" :key="item.id" class="upload-queue__item">
      <div class="upload-queue__head">
        <span class="upload-queue__name" :title="item.name">{{ item.name }}</span>
        <span class="upload-queue__size">{{ formatBytes(item.size) }}</span>
        <NTag size="small" :type="STATE_TAG_TYPE[item.state]" :bordered="false">
          {{ t(`user.upload.${item.state}`) }}
        </NTag>
      </div>
      <div v-if="item.state === 'uploading'" class="upload-queue__progress">
        <NProgress
          class="upload-queue__bar"
          type="line"
          :percentage="progressPercent(item)"
          :show-indicator="false"
        />
        <span class="upload-queue__bytes">
          {{ formatBytes(item.loaded) }} / {{ formatBytes(item.total) }}
        </span>
      </div>
      <div v-if="item.state === 'failed' && item.error" class="upload-queue__error">
        <span class="upload-queue__error-text">
          {{
            t("user.upload.itemError", {
              code: item.error.code,
              error: formatApiError(
                new ApiError(item.error.code, item.error.message, item.error.status),
                "user.upload.failedFallback",
              ),
            })
          }}
        </span>
        <NButton
          size="tiny"
          secondary
          type="warning"
          :disabled="busy"
          @click="emit('retry', item)"
          >{{ t("user.common.retry") }}</NButton
        >
      </div>
      <UploadResultActions v-if="item.state === 'success' && item.image" :image="item.image" />
    </li>
  </ul>
</template>

<style scoped>
.upload-queue {
  list-style: none;
  margin: 0;
  padding: 0;
}

.upload-queue__empty {
  padding: 24px 0;
}

.upload-queue__item {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px 0;
  border-bottom: 1px solid hsl(var(--border));
}

.upload-queue__item:last-child {
  border-bottom: none;
}

.upload-queue__head {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.upload-queue__name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.upload-queue__size {
  flex-shrink: 0;
  font-size: 12px;
  opacity: 0.7;
}

.upload-queue__progress {
  display: flex;
  align-items: center;
  gap: 12px;
}

.upload-queue__bar {
  flex: 1;
}

.upload-queue__bytes {
  flex-shrink: 0;
  font-size: 12px;
  opacity: 0.7;
}

.upload-queue__error {
  display: flex;
  align-items: center;
  gap: 12px;
}

.upload-queue__error-text {
  flex: 1;
  font-size: 13px;
  color: hsl(var(--destructive));
  overflow-wrap: anywhere;
}
@media (max-width: 640px) {
  .upload-queue__head {
    gap: 8px;
  }

  .upload-queue__progress {
    flex-wrap: wrap;
  }

  .upload-queue__bar {
    flex-basis: 100%;
  }
}
</style>
