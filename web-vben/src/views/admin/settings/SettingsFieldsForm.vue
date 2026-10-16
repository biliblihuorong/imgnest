<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { useMediaQuery } from "@vueuse/core";
import {
  NAlert,
  NButton,
  NCard,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NSelect,
  NSwitch,
  useMessage,
  type FormInst,
  type FormItemRule,
  type FormRules,
} from "naive-ui";
import { computed, onMounted, reactive, ref } from "vue";
import {
  getSettings,
  listGroups,
  putSettings,
  type AdminSettings,
  type GroupView,
} from "@/api/admin";
import { formatApiError } from "@/locales/errors";
import { useSiteStore } from "@/stores/site";
import type { SettingsField } from "./fields";

const props = defineProps<{ fields: SettingsField[] }>();
const emit = defineEmits<{ saved: [settings: AdminSettings] }>();

const { t } = useI18n();
const message = useMessage();
const site = useSiteStore();
const isMobile = useMediaQuery("(max-width: 640px)");

const formRef = ref<FormInst | null>(null);
const loading = ref(false);
const loaded = ref(false);
const loadError = ref<unknown>(null);
const saving = ref(false);
const controlsDisabled = computed(() => loading.value || saving.value || !loaded.value);
const groups = ref<GroupView[]>([]);
const keys = computed(() => props.fields.map((field) => field.key));

const model = reactive<AdminSettings>({
  site_name: "",
  registration_enabled: false,
  guest_upload_enabled: false,
  gallery_enabled: false,
  gallery_public_albums_only: false,
  api_enabled: false,
  trash_days: 7,
  guest_group_id: 0,
  default_group_id: 0,
  avatar_provider: "weavatar",
  email_verification_enabled: false,
});

/** 头像服务商固定两项；不提供自由 URL 或更多供应商。 */
const avatarProviderOptions = computed(() => [
  { label: t("admin.settings.avatarWeavatar"), value: "weavatar" },
  { label: t("admin.settings.avatarGravatar"), value: "gravatar" },
]);

/** 用户组选项 + 「未设置（0）」哨兵项（0 = 后端未配置该组）。 */
const groupOptions = computed(() => [
  { label: t("admin.settings.unset"), value: 0 },
  ...groups.value.map((group) => ({ label: group.name, value: group.id })),
]);

async function load(snapshot?: Partial<AdminSettings>): Promise<void> {
  if (loading.value) return;
  loading.value = true;
  if (!snapshot) loaded.value = false;
  loadError.value = null;
  try {
    const settings = await getSettings();
    for (const key of Object.keys(settings) as (keyof AdminSettings)[]) {
      // Preserve any newer input event instead of overwriting it with a late refresh.
      if (!snapshot || !(key in snapshot) || Object.is(model[key], snapshot[key])) {
        Object.assign(model, { [key]: settings[key] });
      }
    }
    loaded.value = true;
  } catch (error) {
    loadError.value = error;
  } finally {
    loading.value = false;
  }
}

async function loadGroups(): Promise<void> {
  try {
    groups.value = await listGroups();
  } catch (error) {
    message.error(() => formatApiError(error, "admin.errors.groupsLoad"));
  }
}

onMounted(() => {
  void load();
  if (props.fields.some((field) => field.kind === "group")) void loadGroups();
});

const rules = computed<FormRules>(() => ({
  site_name: [
    {
      required: true,
      trigger: ["blur", "input"],
      renderMessage: () => t("admin.settings.siteNameRequired"),
      validator: (_rule: FormItemRule, value: string): boolean | Error =>
        value.trim().length > 0 ? true : new Error(t("admin.settings.siteNameRequired")),
    },
  ],
  trash_days: [
    {
      renderMessage: () =>
        t(
          model.trash_days === null
            ? "admin.settings.retentionRequired"
            : "admin.settings.retentionNonNegative",
        ),
      trigger: ["blur", "change"],
      validator: (_rule: FormItemRule, value: number | null): boolean | Error => {
        if (value === null) {
          return new Error(t("admin.settings.retentionRequired"));
        }
        if (value < 0) {
          return new Error(t("admin.settings.retentionNonNegative"));
        }
        return true;
      },
    },
  ],
}));

/** 只提交本页的字段（后端按 PATCH 语义处理），成功后回读刷新。 */
async function save(): Promise<void> {
  if (controlsDisabled.value) {
    return;
  }
  saving.value = true;
  try {
    await formRef.value?.validate();
  } catch {
    // 客户端校验未通过，错误已就地展示
    saving.value = false;
    return;
  }
  try {
    const snapshot: Partial<AdminSettings> = {};
    for (const key of keys.value) Object.assign(snapshot, { [key]: model[key] });
    if ("trash_days" in snapshot) snapshot.trash_days = model.trash_days ?? 0;
    const saved = await putSettings(snapshot);
    message.success(() => t("admin.settings.saved"));
    emit("saved", saved);
    await Promise.all([load(snapshot), site.refresh({ force: true })]);
  } catch (error) {
    message.error(() => formatApiError(error, "admin.errors.save"));
  } finally {
    saving.value = false;
  }
}

defineExpose({ model });
</script>

<template>
  <NCard :bordered="false" class="settings-fields">
    <NAlert v-if="loadError" type="error" class="settings-alert">
      {{ formatApiError(loadError, "admin.errors.settingsLoad") }}
      <NButton
        size="tiny"
        quaternary
        type="primary"
        :disabled="loading || saving"
        @click="load()"
        >{{ t("admin.common.retry") }}</NButton
      >
    </NAlert>
    <NForm
      ref="formRef"
      class="settings-form"
      :model="model"
      :rules="rules"
      :label-placement="isMobile ? 'top' : 'left'"
      :label-width="isMobile ? undefined : 150"
    >
      <NFormItem
        v-for="field in fields"
        :key="field.key"
        :label="t(`admin.settings.${field.label}`)"
        :path="field.key"
      >
        <div class="field">
          <NInput
            v-if="field.kind === 'text'"
            v-model:value="model[field.key] as string"
            class="field-control"
            :placeholder="t(`admin.settings.${field.label}`)"
            :disabled="controlsDisabled"
          />
          <NSwitch
            v-else-if="field.kind === 'switch'"
            v-model:value="model[field.key] as boolean"
            :disabled="controlsDisabled"
          />
          <NInputNumber
            v-else-if="field.kind === 'number'"
            v-model:value="model[field.key] as number"
            class="field-control"
            :min="0"
            :placeholder="t(`admin.settings.${field.label}`)"
            :disabled="controlsDisabled"
          />
          <NSelect
            v-else-if="field.kind === 'group'"
            v-model:value="model[field.key] as number"
            class="field-control"
            :options="groupOptions"
            :placeholder="field.placeholder ? t(`admin.settings.${field.placeholder}`) : undefined"
            :disabled="controlsDisabled"
          />
          <NSelect
            v-else-if="field.kind === 'avatar'"
            v-model:value="model[field.key] as string"
            class="field-control"
            :options="avatarProviderOptions"
            :placeholder="t(`admin.settings.${field.label}`)"
            :disabled="controlsDisabled"
          />
          <p class="field-hint">{{ t(`admin.settings.${field.hint}`) }}</p>
          <slot :name="`after-${field.key}`" :value="model[field.key]" />
        </div>
      </NFormItem>
    </NForm>
    <div class="settings-actions">
      <NButton
        type="primary"
        data-action="save-settings"
        :loading="saving"
        :disabled="controlsDisabled"
        @click="save"
      >
        {{ t("admin.settings.save") }}
      </NButton>
    </div>
  </NCard>
</template>

<style scoped>
.settings-alert {
  margin-bottom: 12px;
}

.settings-form {
  max-width: 640px;
}

.field {
  width: 100%;
}

.field-control {
  width: 100%;
}

.field-hint {
  margin: 4px 0 0;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}

.settings-actions {
  max-width: 640px;
  display: flex;
  justify-content: flex-end;
}
</style>
