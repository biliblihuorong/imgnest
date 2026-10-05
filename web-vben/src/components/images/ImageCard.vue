<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { computed } from "vue";
import {
  NButton,
  NCheckbox,
  NDropdown,
  NImage,
  NPopconfirm,
  NSwitch,
  NTag,
  NTooltip,
  type DropdownOption,
} from "naive-ui";
import type { ImageView } from "@/api/images";
import { formatBytes } from "@/lib/format";
import { formatDateTime } from "./time";
import { useProtectedThumbnail } from "./useProtectedThumbnail";

const { t } = useI18n();

const props = defineProps<{
  image: ImageView;
  /** 该卡片有待完成的写操作（改可见性/删除/移动）时禁用控件。 */
  busy?: boolean;
  /** 是否显示左上角多选框（图片页批量操作）。 */
  selectable?: boolean;
  /** 多选框受控值。 */
  selected?: boolean;
  /** 「移动到相册」下拉选项（key=相册 id，0=移出相册）；为空不显示移动入口。 */
  albumOptions?: DropdownOption[];
}>();

const emit = defineEmits<{
  open: [];
  toggle: [isPublic: boolean];
  remove: [];
  select: [checked: boolean];
  move: [albumId: number];
}>();

const { thumbnailUrl, failed: thumbFailed } = useProtectedThumbnail(
  () => props.image.local_thumb_url,
);
const thumbUrl = computed(() => (thumbFailed.value ? "" : thumbnailUrl.value));
const extText = computed(() => props.image.ext.replace(/^\./, "").toUpperCase());

function onThumbError(): void {
  thumbFailed.value = true;
}

function onMoveSelect(key: string | number): void {
  if (!props.busy) emit("move", Number(key));
}
</script>

<template>
  <div class="image-card">
    <div v-if="selectable" class="image-card__check" @click.stop>
      <NCheckbox
        size="small"
        :checked="selected"
        :disabled="busy"
        :aria-label="t('user.images.selectName', { name: image.name })"
        @update:checked="emit('select', $event)"
      />
    </div>
    <div
      class="image-card__thumb"
      role="button"
      tabindex="0"
      :aria-label="image.name"
      @keydown.enter.prevent="emit('open')"
      @keydown.space.prevent="emit('open')"
      @click="emit('open')"
    >
      <NImage
        v-if="thumbUrl"
        :src="thumbUrl"
        preview-disabled
        object-fit="cover"
        :img-props="{ alt: image.name, onError: onThumbError }"
      />
      <div v-else class="image-card__placeholder">
        <span class="image-card__ext">{{ extText || "IMG" }}</span>
        <span class="image-card__hint">{{ t("user.images.noThumbnail") }}</span>
      </div>
    </div>
    <div class="image-card__body">
      <div class="image-card__name" :title="image.name" @click="emit('open')">
        {{ image.name }}
      </div>
      <div class="image-card__meta">
        <span>{{ formatBytes(image.size) }}</span>
        <span>{{ image.width }}×{{ image.height }}</span>
        <span>{{ formatDateTime(image.created_at) }}</span>
      </div>
      <div class="image-card__actions">
        <NTag size="small" :bordered="false" :type="image.is_public ? 'success' : 'default'">
          {{ t(image.is_public ? "user.common.public" : "user.common.private") }}
        </NTag>
        <NTooltip placement="top-start">
          <template #trigger>
            <NSwitch
              size="small"
              :value="image.is_public"
              :disabled="busy"
              :aria-label="t('user.images.toggleName', { name: image.name })"
              @update:value="emit('toggle', $event)"
            />
          </template>
          {{ t("user.images.privacy") }}
        </NTooltip>
        <div class="image-card__spacer" />
        <NDropdown
          v-if="albumOptions && albumOptions.length > 0"
          trigger="click"
          :options="albumOptions"
          @select="onMoveSelect"
        >
          <NButton size="tiny" quaternary type="primary" :disabled="busy">{{
            t("user.common.move")
          }}</NButton>
        </NDropdown>
        <NPopconfirm
          :positive-text="t('user.common.confirmDelete')"
          :negative-text="t('user.common.cancel')"
          @positive-click="!busy && emit('remove')"
        >
          <template #trigger>
            <NButton size="tiny" quaternary type="error" :disabled="busy">{{
              t("user.common.remove")
            }}</NButton>
          </template>
          {{ t("user.images.deleteConfirm") }}
        </NPopconfirm>
      </div>
    </div>
  </div>
</template>

<style scoped>
.image-card {
  position: relative;
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
  overflow: hidden;
  background: transparent;
}

.image-card__check {
  position: absolute;
  top: 6px;
  left: 6px;
  z-index: 1;
  padding: 2px;
  border-radius: 4px;
  background: hsl(var(--card) / 0.9);
}

.image-card__thumb {
  height: 150px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  background: hsl(var(--muted) / 0.5);
  overflow: hidden;
}

.image-card__thumb :deep(.n-image) {
  width: 100%;
  height: 100%;
}

.image-card__thumb :deep(.n-image img) {
  width: 100%;
  height: 100%;
}

.image-card__placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  color: hsl(var(--muted-foreground));
}

.image-card__ext {
  font-size: 22px;
  font-weight: 600;
  letter-spacing: 1px;
}

.image-card__hint {
  font-size: 12px;
}

.image-card__body {
  padding: 8px 10px 10px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.image-card__name {
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.image-card__meta {
  display: flex;
  gap: 10px;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
  flex-wrap: wrap;
}

.image-card__actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.image-card__spacer {
  flex: 1;
}
</style>
