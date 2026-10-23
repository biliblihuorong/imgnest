<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NButton, useMessage } from "naive-ui";
import { onBeforeUnmount, ref, watch } from "vue";
import { deleteUserPluginAsset, getUserPluginAsset, putUserPluginAsset } from "@/api/plugins";
import { formatApiError } from "@/locales/errors";

const props = defineProps<{ name: string; label: string; hasAsset: boolean; disabled: boolean }>();
const emit = defineEmits<{ busy: [busy: boolean] }>();
const { t } = useI18n();
const message = useMessage();
const preview = ref("");
const hasAsset = ref(props.hasAsset);
const busy = ref(false);
const controller = new AbortController();
let previewRequest: AbortController | undefined;
function clear(): void {
  if (preview.value) URL.revokeObjectURL(preview.value);
  preview.value = "";
}
async function refresh(): Promise<void> {
  previewRequest?.abort();
  clear();
  if (!hasAsset.value) return;
  const request = new AbortController();
  previewRequest = request;
  try {
    const blob = await getUserPluginAsset(props.name, request.signal);
    if (!request.signal.aborted && !controller.signal.aborted)
      preview.value = URL.createObjectURL(blob);
  } catch (error) {
    if (!request.signal.aborted && !controller.signal.aborted)
      message.error(formatApiError(error, "account.plugins.assetError"));
  }
}
watch(
  () => props.hasAsset,
  (value) => {
    hasAsset.value = value;
    void refresh();
  },
  { immediate: true },
);
onBeforeUnmount(() => {
  controller.abort();
  previewRequest?.abort();
  clear();
});
async function change(file?: File): Promise<void> {
  if (busy.value || props.disabled) return;
  if (file && file.size > 4 * 1024 * 1024) {
    message.error(t("account.plugins.tooLarge"));
    return;
  }
  busy.value = true;
  emit("busy", true);
  try {
    const result = file
      ? await putUserPluginAsset(props.name, file, controller.signal)
      : await deleteUserPluginAsset(props.name, controller.signal);
    if (controller.signal.aborted) return;
    hasAsset.value = result.has_asset;
    await refresh();
    message.success(t("account.plugins.assetSaved"));
  } catch (error) {
    if (!controller.signal.aborted)
      message.error(formatApiError(error, "account.plugins.assetError"));
  } finally {
    busy.value = false;
    emit("busy", false);
  }
}
function select(event: Event): void {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (file) void change(file);
  input.value = "";
}
</script>

<template>
  <div class="user-plugin-asset">
    <label
      >{{ label }}
      <input
        type="file"
        accept="image/png,image/jpeg,image/webp"
        :aria-label="label"
        :disabled="disabled || busy"
        @change="select"
    /></label>
    <p>{{ t("account.plugins.assetHint") }}</p>
    <img v-if="preview" :src="preview" :alt="label" />
    <NButton
      v-if="hasAsset"
      size="small"
      attr-type="button"
      :disabled="disabled || busy"
      :loading="busy"
      @click="change()"
      >{{ t("account.plugins.removeAsset") }}</NButton
    >
  </div>
</template>

<style scoped>
.user-plugin-asset {
  display: grid;
  gap: 8px;
  margin-bottom: 20px;
}
.user-plugin-asset label {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
}
.user-plugin-asset p {
  margin: 0;
  color: hsl(var(--muted-foreground));
  font-size: 12px;
}
.user-plugin-asset img {
  max-width: 240px;
  max-height: 160px;
  object-fit: contain;
  background: hsl(var(--muted));
}
</style>
