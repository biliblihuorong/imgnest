<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NAlert, NButton, NCard, NForm, NFormItem, useMessage } from "naive-ui";
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { saveUserPluginSettings, type UserPluginSettings } from "@/api/plugins";
import PluginFieldInput from "@/components/admin/PluginFieldInput.vue";
import { formatApiError } from "@/locales/errors";
import UserPluginAsset from "./UserPluginAsset.vue";

const props = defineProps<{ plugin: UserPluginSettings }>();
const emit = defineEmits<{ saved: [plugin: UserPluginSettings] }>();
const { t } = useI18n();
const message = useMessage();
const model = ref<Record<string, unknown>>({});
const saving = ref(false);
const assetBusy = ref(false);
const disabled = computed(() => props.plugin.read_only || saving.value || assetBusy.value);
const controller = new AbortController();
onBeforeUnmount(() => controller.abort());
watch(
  () => props.plugin.values,
  (values) => {
    model.value = JSON.parse(JSON.stringify(values));
  },
  { immediate: true },
);
async function save(): Promise<void> {
  if (disabled.value) return;
  saving.value = true;
  try {
    const result = await saveUserPluginSettings(props.plugin.name, model.value, controller.signal);
    if (controller.signal.aborted) return;
    emit("saved", result);
    message.success(t("account.plugins.saved"));
  } catch (error) {
    if (!controller.signal.aborted)
      message.error(formatApiError(error, "account.plugins.saveError"));
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <NCard :title="plugin.title" :bordered="false" class="mt-5" :data-user-plugin="plugin.name">
    <p v-if="plugin.description" class="description">{{ plugin.description }}</p>
    <NAlert v-if="plugin.notice" class="notice" type="info">{{ plugin.notice }}</NAlert>
    <NForm label-placement="top" @submit.prevent="save">
      <NFormItem
        v-for="field in plugin.fields"
        :key="field.key"
        :label="field.label"
        :required="field.required"
      >
        <PluginFieldInput v-model="model[field.key]" :field="field" :disabled="disabled" />
      </NFormItem>
      <UserPluginAsset
        v-if="plugin.asset_label && !plugin.read_only"
        :name="plugin.name"
        :label="plugin.asset_label"
        :has-asset="plugin.has_asset"
        :disabled="saving"
        @busy="assetBusy = $event"
      />
      <NButton
        v-if="!plugin.read_only"
        type="primary"
        attr-type="button"
        data-action="save-user-plugin"
        :loading="saving"
        :disabled="disabled"
        @click="save"
        >{{ t("account.plugins.save") }}</NButton
      >
    </NForm>
  </NCard>
</template>

<style scoped>
.description {
  color: hsl(var(--muted-foreground));
  margin: 0 0 16px;
}
.notice {
  margin-bottom: 16px;
}
</style>
