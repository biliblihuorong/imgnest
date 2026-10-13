<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { useMediaQuery } from "@vueuse/core";
import {
  NAlert,
  NButton,
  NCard,
  NForm,
  NFormItem,
  NTag,
  useMessage,
  type SelectOption,
} from "naive-ui";
import { computed, ref, shallowRef, watch } from "vue";
import PluginFieldInput from "@/components/admin/PluginFieldInput.vue";
import { formatApiError } from "@/locales/errors";
import {
  ITEM_KEY,
  savePluginSettings,
  type PluginSettingField,
  type PluginSettings,
} from "@/api/plugins";

type Values = Record<string, unknown>;

const props = defineProps<{
  plugin: PluginSettings;
  /** OptionsFrom 来源（groups / policies / storages）对应的选项。 */
  sources: Record<string, SelectOption[]>;
}>();
const emit = defineEmits<{ saved: [plugin: PluginSettings] }>();

const { t } = useI18n();
const message = useMessage();
const isMobile = useMediaQuery("(max-width: 640px)");

const model = ref<Values>({});
const saving = shallowRef(false);
/** 保存后递增，让字段组件按新值重新挂载（密钥占位状态随之重置）。 */
const generation = shallowRef(0);

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value ?? null)) as T;
}

watch(
  () => props.plugin.values,
  (values) => {
    model.value = clone(values) ?? {};
    generation.value += 1;
  },
  { immediate: true },
);

const hasSecrets = computed(() =>
  props.plugin.fields.some(
    (field) =>
      field.type === "secret" ||
      (field.type === "list" && (field.fields ?? []).some((item) => item.type === "secret")),
  ),
);

const statusType = (level: string) =>
  (({ success: "success", warning: "warning", error: "error" }) as Record<string, string>)[level] ??
  "default";

function defaultValue(field: PluginSettingField): unknown {
  if (field.default !== undefined && field.default !== null) return clone(field.default);
  switch (field.type) {
    case "bool":
      return false;
    case "int":
    case "number":
      return field.min ?? 0;
    case "tags":
    case "list":
      return [];
    case "select":
      return field.multiple ? [] : null;
    default:
      return "";
  }
}

function items(field: PluginSettingField): Values[] {
  const value = model.value[field.key];
  return Array.isArray(value) ? (value as Values[]) : [];
}

function addItem(field: PluginSettingField): void {
  const item: Values = {};
  for (const child of field.fields ?? []) item[child.key] = defaultValue(child);
  model.value[field.key] = [...items(field), item];
}

function removeItem(field: PluginSettingField, index: number): void {
  model.value[field.key] = items(field).filter((_, i) => i !== index);
}

function itemTitle(field: PluginSettingField, item: Values, index: number): string {
  const label = field.item_label ? item[field.item_label] : undefined;
  return typeof label === "string" && label.trim()
    ? label
    : t("admin.plugins.itemTitle", { index: index + 1 });
}

function itemKey(item: Values, index: number): string {
  return typeof item[ITEM_KEY] === "string" ? (item[ITEM_KEY] as string) : `new-${index}`;
}

async function save(): Promise<void> {
  if (saving.value) return;
  saving.value = true;
  try {
    const saved = await savePluginSettings(props.plugin.name, model.value);
    message.success(() => t("admin.plugins.saved", { title: props.plugin.title }));
    emit("saved", saved);
  } catch (error) {
    message.error(() => formatApiError(error, "admin.errors.save"));
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <NCard :title="plugin.title" class="plugin-card" :data-plugin="plugin.name">
    <template #header-extra>
      <NButton
        type="primary"
        size="small"
        attr-type="button"
        data-action="save"
        :loading="saving"
        @click="save"
        >{{ t("admin.plugins.save") }}</NButton
      >
    </template>
    <p v-if="plugin.description" class="description">{{ plugin.description }}</p>
    <div v-if="plugin.status.length" class="status-list" data-plugin-status>
      <div v-for="line in plugin.status" :key="line.label" class="status-line">
        <span class="status-label">{{ line.label }}</span>
        <NTag size="small" :type="statusType(line.level) as any">{{ line.value }}</NTag>
      </div>
    </div>
    <NAlert v-if="hasSecrets && !plugin.secrets_available" type="warning" class="notice">
      {{ t("admin.plugins.secretsUnavailable") }}
    </NAlert>
    <NForm
      :key="generation"
      class="plugin-form"
      :label-placement="isMobile ? 'top' : 'left'"
      :label-width="isMobile ? undefined : 150"
      @submit.prevent="save"
    >
      <template v-for="field in plugin.fields" :key="field.key">
        <NFormItem
          v-if="field.type !== 'list'"
          :label="field.label"
          :required="field.required"
          :path="field.key"
        >
          <PluginFieldInput
            v-model="model[field.key]"
            :field="field"
            :source-options="field.options_from ? sources[field.options_from] : undefined"
            :disabled="saving"
          />
        </NFormItem>
        <NFormItem v-else :label="field.label" :required="field.required" :path="field.key">
          <div class="list-field" :data-list="field.key">
            <p v-if="field.help" class="description">{{ field.help }}</p>
            <p v-if="!items(field).length" class="description">
              {{ t("admin.plugins.emptyList") }}
            </p>
            <div
              v-for="(item, index) in items(field)"
              :key="itemKey(item, index)"
              class="list-item"
              data-list-item
            >
              <div class="list-item-header">
                <strong>{{ itemTitle(field, item, index) }}</strong>
                <NButton
                  size="tiny"
                  quaternary
                  type="error"
                  attr-type="button"
                  data-action="remove-item"
                  :disabled="saving"
                  @click="removeItem(field, index)"
                  >{{ t("admin.plugins.removeItem") }}</NButton
                >
              </div>
              <NFormItem
                v-for="child in field.fields ?? []"
                :key="child.key"
                :label="child.label"
                :required="child.required"
                label-placement="top"
              >
                <PluginFieldInput
                  v-model="item[child.key]"
                  :field="child"
                  :source-options="child.options_from ? sources[child.options_from] : undefined"
                  :disabled="saving"
                />
              </NFormItem>
            </div>
            <NButton
              size="small"
              dashed
              attr-type="button"
              data-action="add-item"
              :disabled="saving"
              @click="addItem(field)"
              >{{ t("admin.plugins.addItem") }}</NButton
            >
          </div>
        </NFormItem>
      </template>
    </NForm>
  </NCard>
</template>

<style scoped>
.plugin-card {
  margin-top: 20px;
  min-width: 0;
}

.plugin-form {
  max-width: 720px;
}

.description {
  margin: 0 0 12px;
  color: hsl(var(--muted-foreground));
  overflow-wrap: anywhere;
}

.notice {
  margin: 0 0 16px;
}

.status-list {
  display: grid;
  gap: 8px;
  margin: 0 0 16px;
}

.status-line {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.status-label {
  min-width: 96px;
  color: hsl(var(--muted-foreground));
}

.list-field {
  display: grid;
  gap: 12px;
  width: 100%;
  min-width: 0;
}

.list-item {
  padding: 12px 14px 0;
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
}

.list-item-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 8px;
  overflow-wrap: anywhere;
}
</style>
