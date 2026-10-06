<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { computed, onBeforeUnmount, ref, watch } from "vue";
import {
  NCollapse,
  NCollapseItem,
  NDescriptions,
  NDescriptionsItem,
  NDrawer,
  NDrawerContent,
  NEmpty,
  NImage,
  NTag,
  useMessage,
} from "naive-ui";
import { getImageExif, type ImageExif, type ImageView } from "@/api/images";
import { formatBytes } from "@/lib/format";
import { formatDateTime } from "./time";
import { formatApiError } from "@/locales/errors";

const { t } = useI18n();

const props = defineProps<{
  show: boolean;
  image: ImageView | null;
}>();

const emit = defineEmits<{
  "update:show": [value: boolean];
}>();

const message = useMessage();

let requestId = 0;
onBeforeUnmount(() => {
  requestId++;
});

const exif = ref<ImageExif | null>(null);
const exifLoading = ref(false);
const exifFailed = ref(false);

const typeText = computed(() =>
  props.image ? `${props.image.ext.replace(/^\./, "").toUpperCase()}（${props.image.mime}）` : "-",
);
const hasGps = computed(
  () => exif.value !== null && (exif.value.gps_lat !== null || exif.value.gps_lng !== null),
);
const rawText = computed(() => (exif.value ? JSON.stringify(exif.value.raw, null, 2) : ""));

/** 抽屉顶部大图：WebP 优先，点击进入全屏灯箱。 */
const previewSrc = computed(() => {
  const image = props.image;
  if (!image) return "";
  return image.links.webp || image.links.original || image.links.url;
});

watch(
  () => [props.show, props.image?.id] as const,
  ([show, id]) => {
    if (show && id !== undefined) {
      void loadExif(id);
      return;
    }
    if (!show) {
      requestId++;
      exifLoading.value = false;
      // 关闭时清空，下次打开重新懒加载最新数据
      exif.value = null;
      exifFailed.value = false;
    }
  },
  { immediate: true },
);

async function loadExif(id: number): Promise<void> {
  const current = ++requestId;
  exifLoading.value = true;
  exifFailed.value = false;
  exif.value = null;
  try {
    const result = await getImageExif(id);
    if (current === requestId && props.show) exif.value = result;
  } catch (error) {
    if (current !== requestId || !props.show) return;
    exifFailed.value = true;
    message.error(() => formatApiError(error, "user.detail.exifError"));
  } finally {
    if (current === requestId) exifLoading.value = false;
  }
}

function handleClose(value: boolean): void {
  emit("update:show", value);
}
</script>

<template>
  <NDrawer :show="props.show" width="min(520px, 100vw)" @update:show="handleClose">
    <NDrawerContent :title="props.image?.name ?? t('user.detail.title')" closable>
      <template v-if="props.image">
        <NImage
          class="drawer-preview"
          :src="previewSrc"
          :preview-src="previewSrc"
          object-fit="contain"
          :img-props="{ alt: props.image.name }"
        />
        <NDescriptions bordered :column="1" size="small" label-placement="left">
          <NDescriptionsItem :label="t('user.common.name')">{{
            props.image.name
          }}</NDescriptionsItem>
          <NDescriptionsItem :label="t('user.detail.dimensions')">
            {{ props.image.width }} × {{ props.image.height }}
            <span v-if="props.image.frames > 1">{{
              t("user.detail.frames", { count: props.image.frames })
            }}</span>
          </NDescriptionsItem>
          <NDescriptionsItem :label="t('user.detail.type')">{{ typeText }}</NDescriptionsItem>
          <NDescriptionsItem :label="t('user.common.size')">{{
            formatBytes(props.image.size)
          }}</NDescriptionsItem>
          <NDescriptionsItem :label="t('user.detail.webpSize')">
            {{ formatBytes(props.image.webp_size) }}
          </NDescriptionsItem>
          <NDescriptionsItem :label="t('user.detail.chargedSize')">
            {{ formatBytes(props.image.charged_bytes) }}
          </NDescriptionsItem>
          <NDescriptionsItem :label="t('user.detail.created')">
            {{ formatDateTime(props.image.created_at) }}
          </NDescriptionsItem>
          <NDescriptionsItem label="src_md5">{{ props.image.src_md5 }}</NDescriptionsItem>
        </NDescriptions>

        <div class="drawer-section">
          <div class="drawer-section-title">{{ t("user.detail.exif") }}</div>
          <p v-if="exifLoading" class="drawer-hint">{{ t("user.detail.exifLoading") }}</p>
          <template v-else-if="exif">
            <NDescriptions bordered :column="1" size="small" label-placement="left">
              <NDescriptionsItem :label="t('user.detail.camera')">
                {{ [exif.make, exif.model].filter(Boolean).join(" ") || "-" }}
              </NDescriptionsItem>
              <NDescriptionsItem :label="t('user.detail.lens')">{{
                exif.lens || "-"
              }}</NDescriptionsItem>
              <NDescriptionsItem :label="t('user.detail.exposure')">{{
                exif.exposure || "-"
              }}</NDescriptionsItem>
              <NDescriptionsItem :label="t('user.detail.aperture')">{{
                exif.f_number || "-"
              }}</NDescriptionsItem>
              <NDescriptionsItem :label="t('user.detail.focal')">{{
                exif.focal_length || "-"
              }}</NDescriptionsItem>
              <NDescriptionsItem label="ISO">{{ exif.iso > 0 ? exif.iso : "-" }}</NDescriptionsItem>
              <NDescriptionsItem :label="t('user.detail.orientation')">
                {{ exif.orientation > 0 ? exif.orientation : "-" }}
              </NDescriptionsItem>
              <NDescriptionsItem :label="t('user.detail.taken')">
                {{ formatDateTime(exif.taken_at) }}
              </NDescriptionsItem>
            </NDescriptions>

            <div class="drawer-gps">
              <div class="drawer-gps-title">
                <span>{{ t("user.detail.gps") }}</span>
                <NTag size="small" type="warning" :bordered="false">{{
                  t("user.detail.ownerOnly")
                }}</NTag>
              </div>
              <p v-if="hasGps" class="drawer-gps-value">
                {{
                  t("user.detail.coordinates", {
                    lat: exif.gps_lat ?? "-",
                    lng: exif.gps_lng ?? "-",
                    alt: exif.gps_alt ?? "-",
                  })
                }}
              </p>
              <p v-else class="drawer-hint">{{ t("user.detail.noGps") }}</p>
            </div>

            <NCollapse>
              <NCollapseItem :title="t('user.detail.raw')" name="raw">
                <pre class="drawer-raw">{{ rawText }}</pre>
              </NCollapseItem>
            </NCollapse>
          </template>
          <NEmpty v-else-if="!exifFailed" size="small" :description="t('user.detail.noExif')" />
          <p v-else class="drawer-hint">{{ t("user.detail.exifRetry") }}</p>
        </div>
      </template>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.drawer-preview {
  display: flex;
  justify-content: center;
  margin-bottom: 16px;
  padding: 8px;
  border-radius: 6px;
  background: hsl(var(--muted) / 0.5);
}

.drawer-preview :deep(img) {
  max-height: 320px;
}

.drawer-section {
  margin-top: 16px;
}

.drawer-section-title {
  font-weight: 600;
  margin-bottom: 8px;
}

.drawer-gps {
  margin-top: 12px;
}

.drawer-gps-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 500;
}

.drawer-gps-value {
  margin: 6px 0 0;
  font-size: 13px;
}

.drawer-hint {
  margin: 6px 0 0;
  font-size: 13px;
  color: rgba(128, 128, 128, 0.9);
}

.drawer-raw {
  margin: 0;
  padding: 8px;
  font-size: 12px;
  line-height: 1.5;
  overflow: auto;
  max-height: 320px;
  background: rgba(128, 128, 128, 0.08);
  border-radius: 4px;
}
</style>
