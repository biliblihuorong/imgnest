<script setup lang="ts">
import { X } from "@vben/icons";
import { useI18n } from "@vben/locales";
import type { ImageView } from "@/api/images";
import ImageDetailContent from "./ImageDetailContent.vue";

/** Marvis 宽屏下常驻在图片库右侧的详情面板；内容与详情抽屉相同。 */
const props = defineProps<{ image: ImageView | null }>();
const emit = defineEmits<{ close: [] }>();
const { t } = useI18n();
</script>

<template>
  <aside class="image-detail-panel" :aria-label="t('user.detail.title')">
    <template v-if="props.image">
      <header class="image-detail-panel__head">
        <span
          class="image-detail-panel__title"
          data-testid="detail-panel-title"
          :title="props.image.name"
          >{{ props.image.name }}</span
        >
        <button
          type="button"
          class="image-detail-panel__close"
          data-testid="detail-panel-close"
          :aria-label="t('shell.detailPanel.close')"
          @click="emit('close')"
        >
          <X class="image-detail-panel__icon" />
        </button>
      </header>
      <div class="image-detail-panel__body">
        <ImageDetailContent :image="props.image" active />
      </div>
    </template>
    <div v-else class="image-detail-panel__empty" data-testid="detail-panel-empty">
      {{ t("shell.detailPanel.empty") }}
    </div>
  </aside>
</template>

<style scoped>
.image-detail-panel {
  position: sticky;
  top: 0;
  display: flex;
  flex-shrink: 0;
  flex-direction: column;
  align-self: flex-start;
  width: 320px;
  max-height: calc(100vh - 32px);
  background: hsl(var(--card));
  border: 1px solid hsl(var(--border));
  border-radius: 16px;
}
.image-detail-panel__head {
  display: flex;
  gap: 8px;
  align-items: center;
  padding: 14px 14px 10px 18px;
}
.image-detail-panel__title {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  font-size: 15px;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.image-detail-panel__close {
  display: grid;
  flex-shrink: 0;
  place-items: center;
  width: 29px;
  height: 29px;
  color: hsl(var(--muted-foreground));
  border-radius: 8px;
}
.image-detail-panel__close:hover {
  background: hsl(var(--muted));
}
.image-detail-panel__icon {
  width: 16px;
  height: 16px;
}
.image-detail-panel__body {
  min-height: 0;
  padding: 0 16px 16px;
  overflow-y: auto;
}
.image-detail-panel__empty {
  display: grid;
  place-items: center;
  min-height: 240px;
  padding: 24px;
  font-size: 13px;
  color: hsl(var(--muted-foreground));
  text-align: center;
}
</style>
