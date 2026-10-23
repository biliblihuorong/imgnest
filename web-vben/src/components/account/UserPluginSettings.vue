<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NAlert, NButton, NSpin } from "naive-ui";
import { onBeforeUnmount, ref, watch } from "vue";
import { listUserPluginSettings, type UserPluginSettings } from "@/api/plugins";
import { formatApiError } from "@/locales/errors";
import { useAuthStore } from "@/stores/auth";
import UserPluginCard from "./UserPluginCard.vue";

const { t } = useI18n();
const auth = useAuthStore();
const plugins = ref<UserPluginSettings[]>([]);
const error = ref("");
const loading = ref(false);
let controller: AbortController | undefined;
async function load(): Promise<void> {
  controller?.abort();
  const request = new AbortController();
  controller = request;
  plugins.value = [];
  error.value = "";
  loading.value = false;
  if (!auth.user || !auth.token) return;
  loading.value = true;
  try {
    const result = await listUserPluginSettings(request.signal);
    if (!request.signal.aborted) plugins.value = result;
  } catch (cause) {
    if (!request.signal.aborted) error.value = formatApiError(cause, "account.plugins.loadError");
  } finally {
    if (!request.signal.aborted) loading.value = false;
  }
}
watch(
  () => [auth.token, auth.user?.id, auth.user?.group_id],
  () => void load(),
  { immediate: true, flush: "sync" },
);
onBeforeUnmount(() => controller?.abort());
function saved(plugin: UserPluginSettings): void {
  plugins.value = plugins.value.map((item) => (item.name === plugin.name ? plugin : item));
}
</script>

<template>
  <NSpin v-if="loading" class="mt-5" />
  <NAlert v-if="error" type="error" class="mt-5">
    {{ error }} <NButton size="small" @click="load">{{ t("account.retry") }}</NButton>
  </NAlert>
  <UserPluginCard
    v-for="plugin in plugins"
    :key="`${auth.user?.id}:${auth.user?.group_id}:${plugin.name}`"
    :plugin="plugin"
    @saved="saved"
  />
</template>
