<script setup lang="ts">
// 扩展插件的设置卡片。社区版没有插件，列表为空时本组件不渲染任何内容。
import { useI18n } from "@vben/locales";
import { NAlert, NButton, type SelectOption } from "naive-ui";
import { onMounted, ref, shallowRef } from "vue";
import PluginSettingsCard from "@/components/admin/PluginSettingsCard.vue";
import { listGroups, listPolicies, listStorages } from "@/api/admin";
import { listPluginSettings, type PluginSettings } from "@/api/plugins";
import { formatApiError } from "@/locales/errors";

const { t } = useI18n();
const plugins = ref<PluginSettings[]>([]);
const loadError = shallowRef<unknown>(null);
const loading = shallowRef(false);
const sources = ref<Record<string, SelectOption[]>>({});

function usedSources(list: PluginSettings[]): Set<string> {
  const used = new Set<string>();
  for (const plugin of list) {
    for (const field of plugin.fields) {
      if (field.options_from) used.add(field.options_from);
      for (const child of field.fields ?? []) if (child.options_from) used.add(child.options_from);
    }
  }
  return used;
}

async function loadSources(used: Set<string>): Promise<void> {
  const loaders: Record<string, () => Promise<SelectOption[]>> = {
    groups: async () =>
      (await listGroups()).map((group) => ({ label: group.name, value: group.id })),
    policies: async () =>
      (await listPolicies()).map((policy) => ({ label: policy.name, value: policy.id })),
    storages: async () =>
      (await listStorages()).map((storage) => ({ label: storage.name, value: storage.id })),
  };
  await Promise.all(
    [...used].map(async (source) => {
      try {
        sources.value = { ...sources.value, [source]: await (loaders[source]?.() ?? []) };
      } catch {
        // 选项加载失败时仍可编辑其它设置；下拉框为空。
      }
    }),
  );
}

async function load(): Promise<void> {
  if (loading.value) return;
  loading.value = true;
  loadError.value = null;
  try {
    plugins.value = await listPluginSettings();
    await loadSources(usedSources(plugins.value));
  } catch (error) {
    loadError.value = error;
  } finally {
    loading.value = false;
  }
}

function replace(saved: PluginSettings): void {
  plugins.value = plugins.value.map((plugin) => (plugin.name === saved.name ? saved : plugin));
}

onMounted(() => void load());
</script>

<template>
  <section v-if="plugins.length || loadError" class="plugin-settings" data-plugin-settings>
    <h2 class="section-title">{{ t("admin.plugins.title") }}</h2>
    <p class="section-description">{{ t("admin.plugins.description") }}</p>
    <NAlert v-if="loadError" type="error">
      {{ formatApiError(loadError, "admin.plugins.loadError") }}
      <NButton size="tiny" quaternary type="primary" :disabled="loading" @click="load">{{
        t("admin.common.retry")
      }}</NButton>
    </NAlert>
    <PluginSettingsCard
      v-for="plugin in plugins"
      :key="plugin.name"
      :plugin="plugin"
      :sources="sources"
      @saved="replace"
    />
  </section>
</template>

<style scoped>
.plugin-settings {
  margin-top: 32px;
}

.section-title {
  margin: 0 0 4px;
  font-size: 18px;
  font-weight: 600;
}

.section-description {
  margin: 0 0 4px;
  color: hsl(var(--muted-foreground));
}
</style>
