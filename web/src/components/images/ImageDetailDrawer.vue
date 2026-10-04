<script setup lang="ts">
import { computed, ref, watch } from "vue";
import {
  NCollapse,
  NCollapseItem,
  NDescriptions,
  NDescriptionsItem,
  NDrawer,
  NDrawerContent,
  NEmpty,
  NTag,
  useMessage,
} from "naive-ui";
import { getImageExif, type ImageExif, type ImageView } from "@/api/images";
import { formatBytes } from "@/lib/format";
import { formatDateTime } from "./time";
import { apiErrorMessage } from "./error";

const props = defineProps<{
  show: boolean;
  image: ImageView | null;
}>();

const emit = defineEmits<{
  "update:show": [value: boolean];
}>();

const message = useMessage();

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

watch(
  () => [props.show, props.image?.id] as const,
  ([show, id]) => {
    if (show && id !== undefined) {
      void loadExif(id);
      return;
    }
    if (!show) {
      // 关闭时清空，下次打开重新懒加载最新数据
      exif.value = null;
      exifFailed.value = false;
    }
  },
  { immediate: true },
);

async function loadExif(id: number): Promise<void> {
  exifLoading.value = true;
  exifFailed.value = false;
  exif.value = null;
  try {
    exif.value = await getImageExif(id);
  } catch (error) {
    exifFailed.value = true;
    message.error(`EXIF 加载失败：${apiErrorMessage(error)}`);
  } finally {
    exifLoading.value = false;
  }
}

function handleClose(value: boolean): void {
  emit("update:show", value);
}
</script>

<template>
  <NDrawer :show="props.show" :width="520" @update:show="handleClose">
    <NDrawerContent :title="props.image?.name ?? '图片详情'" closable>
      <template v-if="props.image">
        <NDescriptions bordered :column="1" size="small" label-placement="left">
          <NDescriptionsItem label="名称">{{ props.image.name }}</NDescriptionsItem>
          <NDescriptionsItem label="尺寸">
            {{ props.image.width }} × {{ props.image.height }}
            <span v-if="props.image.frames > 1">（{{ props.image.frames }} 帧）</span>
          </NDescriptionsItem>
          <NDescriptionsItem label="类型">{{ typeText }}</NDescriptionsItem>
          <NDescriptionsItem label="大小">{{ formatBytes(props.image.size) }}</NDescriptionsItem>
          <NDescriptionsItem label="WebP 大小">
            {{ formatBytes(props.image.webp_size) }}
          </NDescriptionsItem>
          <NDescriptionsItem label="计费大小">
            {{ formatBytes(props.image.charged_bytes) }}
          </NDescriptionsItem>
          <NDescriptionsItem label="上传时间">
            {{ formatDateTime(props.image.created_at) }}
          </NDescriptionsItem>
          <NDescriptionsItem label="src_md5">{{ props.image.src_md5 }}</NDescriptionsItem>
        </NDescriptions>

        <div class="drawer-section">
          <div class="drawer-section-title">EXIF 信息</div>
          <p v-if="exifLoading" class="drawer-hint">EXIF 加载中…</p>
          <template v-else-if="exif">
            <NDescriptions bordered :column="1" size="small" label-placement="left">
              <NDescriptionsItem label="相机">
                {{ [exif.make, exif.model].filter(Boolean).join(" ") || "-" }}
              </NDescriptionsItem>
              <NDescriptionsItem label="镜头">{{ exif.lens || "-" }}</NDescriptionsItem>
              <NDescriptionsItem label="曝光时间">{{ exif.exposure || "-" }}</NDescriptionsItem>
              <NDescriptionsItem label="光圈">{{ exif.f_number || "-" }}</NDescriptionsItem>
              <NDescriptionsItem label="焦距">{{ exif.focal_length || "-" }}</NDescriptionsItem>
              <NDescriptionsItem label="ISO">{{ exif.iso > 0 ? exif.iso : "-" }}</NDescriptionsItem>
              <NDescriptionsItem label="方向">
                {{ exif.orientation > 0 ? exif.orientation : "-" }}
              </NDescriptionsItem>
              <NDescriptionsItem label="拍摄时间">
                {{ formatDateTime(exif.taken_at) }}
              </NDescriptionsItem>
            </NDescriptions>

            <div class="drawer-gps">
              <div class="drawer-gps-title">
                <span>GPS 定位</span>
                <NTag size="small" type="warning" :bordered="false">仅本人可见</NTag>
              </div>
              <p v-if="hasGps" class="drawer-gps-value">
                纬度 {{ exif.gps_lat ?? "-" }} · 经度 {{ exif.gps_lng ?? "-" }} · 高度
                {{ exif.gps_alt ?? "-" }} m
              </p>
              <p v-else class="drawer-hint">无 GPS 信息</p>
            </div>

            <NCollapse>
              <NCollapseItem title="原始元数据（raw）" name="raw">
                <pre class="drawer-raw">{{ rawText }}</pre>
              </NCollapseItem>
            </NCollapse>
          </template>
          <NEmpty
            v-else-if="!exifFailed"
            size="small"
            description="该图片没有 EXIF 信息"
          />
          <p v-else class="drawer-hint">EXIF 加载失败，可关闭抽屉后重试。</p>
        </div>
      </template>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
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
