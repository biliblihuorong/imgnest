<script setup lang="ts">
import { useI18n } from "@vben/locales";
import {
  NCheckbox,
  NDynamicTags,
  NInput,
  NInputNumber,
  NSelect,
  NSwitch,
  type SelectOption,
} from "naive-ui";
import { computed, ref } from "vue";
import { SECRET_KEPT, type PluginSettingField } from "@/api/plugins";

const props = defineProps<{
  field: PluginSettingField;
  modelValue: unknown;
  /** OptionsFrom 字段的选项（组、规则、存储），由父组件按需加载。 */
  sourceOptions?: SelectOption[];
  disabled?: boolean;
}>();
const emit = defineEmits<{ "update:modelValue": [value: unknown] }>();
const { t } = useI18n();

const options = computed<SelectOption[]>(() =>
  props.field.options_from
    ? (props.sourceOptions ?? [])
    : (props.field.options ?? []).map((option) => ({
        label: option.label,
        value: option.value,
      })),
);

/** 已保存的密钥只以占位符出现；输入框留空即保持不变。保存后父组件重新挂载本组件。 */
const wasKept = ref(props.modelValue === SECRET_KEPT);
const secretKept = computed(() => props.modelValue === SECRET_KEPT);
const secretCleared = computed(() => wasKept.value && props.modelValue === "");
const secretText = computed(() =>
  secretKept.value || secretCleared.value ? "" : String(props.modelValue ?? ""),
);

function updateSecret(value: string): void {
  emit("update:modelValue", value === "" && wasKept.value ? SECRET_KEPT : value);
}

function clearSecret(checked: boolean): void {
  emit("update:modelValue", checked ? "" : SECRET_KEPT);
}

const numberValue = computed(() => {
  const value = Number(props.modelValue);
  return Number.isFinite(value) ? value : null;
});

const stringList = computed(() =>
  Array.isArray(props.modelValue) ? (props.modelValue as unknown[]).map(String) : [],
);
</script>

<template>
  <div class="plugin-field">
    <NSwitch
      v-if="field.type === 'bool'"
      :value="modelValue === true"
      :disabled="disabled"
      @update:value="emit('update:modelValue', $event)"
    />
    <NInput
      v-else-if="field.type === 'text'"
      :value="String(modelValue ?? '')"
      :placeholder="field.placeholder"
      :disabled="disabled"
      :input-props="{ 'aria-label': field.label }"
      @update:value="emit('update:modelValue', $event)"
    />
    <NInput
      v-else-if="field.type === 'textarea'"
      type="textarea"
      :autosize="{ minRows: 3, maxRows: 12 }"
      :value="String(modelValue ?? '')"
      :placeholder="field.placeholder"
      :disabled="disabled"
      :input-props="{ 'aria-label': field.label }"
      @update:value="emit('update:modelValue', $event)"
    />
    <template v-else-if="field.type === 'secret'">
      <NInput
        type="password"
        show-password-on="click"
        :value="secretText"
        :placeholder="wasKept ? t('admin.plugins.secretKept') : field.placeholder"
        :disabled="disabled || secretCleared"
        :input-props="{ 'aria-label': field.label, autocomplete: 'new-password' }"
        @update:value="updateSecret"
      />
      <NCheckbox
        v-if="wasKept && (secretKept || secretCleared)"
        class="secret-clear"
        :checked="secretCleared"
        :disabled="disabled"
        @update:checked="clearSecret"
        >{{ t("admin.plugins.secretClear") }}</NCheckbox
      >
    </template>
    <NInputNumber
      v-else-if="field.type === 'int' || field.type === 'number'"
      class="plugin-field-control"
      :value="numberValue"
      :min="field.min"
      :max="field.max"
      :precision="field.type === 'int' ? 0 : undefined"
      :step="field.type === 'int' ? 1 : 0.01"
      :placeholder="field.placeholder"
      :disabled="disabled"
      :input-props="{ 'aria-label': field.label }"
      @update:value="emit('update:modelValue', $event ?? field.min ?? 0)"
    />
    <NSelect
      v-else-if="field.type === 'select'"
      :value="(modelValue as any) ?? null"
      :options="options"
      :multiple="field.multiple"
      :clearable="!field.required"
      :placeholder="field.placeholder"
      :disabled="disabled"
      @update:value="emit('update:modelValue', $event ?? (field.multiple ? [] : null))"
    />
    <NDynamicTags
      v-else-if="field.type === 'tags'"
      :value="stringList"
      :disabled="disabled"
      @update:value="emit('update:modelValue', $event)"
    />
    <p v-if="field.help" class="field-hint">{{ field.help }}</p>
  </div>
</template>

<style scoped>
.plugin-field {
  width: 100%;
}

.plugin-field-control {
  width: 100%;
}

.secret-clear {
  margin-top: 6px;
}

.field-hint {
  margin: 4px 0 0;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}
</style>
