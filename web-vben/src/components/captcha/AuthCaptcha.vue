<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NButton } from "naive-ui";
import { computed, onBeforeUnmount, onMounted, shallowRef, watch } from "vue";
import { fetchCaptcha, type CaptchaAction, type PublicCaptchaConfig } from "@/api/captcha";
import CaptchaChallenge from "./CaptchaChallenge.vue";

const props = defineProps<{ action: CaptchaAction }>();
const emit = defineEmits<{ ready: [value: boolean] }>();
const { t } = useI18n();
const config = shallowRef<PublicCaptchaConfig | null>(null);
const loading = shallowRef(true);
const failed = shallowRef(false);
const token = shallowRef<string | null>(null);
const resetKey = shallowRef(0);
let controller: AbortController | undefined;
let disposed = false;
const ready = computed(
  () =>
    !loading.value &&
    !failed.value &&
    config.value !== null &&
    (!config.value.enabled || !!token.value),
);
watch(ready, (value) => emit("ready", value), { immediate: true, flush: "sync" });

async function reload(): Promise<void> {
  controller?.abort();
  controller = new AbortController();
  const request = controller;
  token.value = null;
  resetKey.value++;
  loading.value = true;
  failed.value = false;
  config.value = null;
  try {
    const next = await fetchCaptcha(request.signal);
    if (!disposed && controller === request) config.value = next;
  } catch {
    if (!disposed && controller === request) failed.value = true;
  } finally {
    if (!disposed && controller === request) loading.value = false;
  }
}
/** Caller checks ready; clearing is synchronous so a token cannot be consumed twice. */
function consume(): string | undefined {
  if (!ready.value) throw new Error("captcha-not-ready");
  if (!config.value?.enabled) return undefined;
  const result = token.value!;
  token.value = null;
  resetKey.value++;
  return result;
}
onMounted(() => {
  void reload();
});
onBeforeUnmount(() => {
  disposed = true;
  controller?.abort();
  token.value = null;
});
defineExpose({ consume, reload });
</script>

<template>
  <div v-if="loading" role="status" aria-live="polite" class="captcha-state">
    {{ t("captcha.configLoading") }}
  </div>
  <div v-else-if="failed" role="alert" class="captcha-state">
    <p>{{ t("captcha.configUnavailable") }}</p>
    <NButton attr-type="button" size="small" @click="reload">{{ t("captcha.retry") }}</NButton>
  </div>
  <CaptchaChallenge
    v-else-if="config?.enabled"
    :config="config"
    :action="props.action"
    :reset-key="resetKey"
    @token="token = $event"
  />
</template>

<style scoped>
.captcha-state {
  margin-bottom: 18px;
  font-size: 13px;
  overflow-wrap: anywhere;
}
</style>
