<script setup lang="ts">
import CaptchaSettingsCard from "@/components/admin/CaptchaSettingsCard.vue";
import { Page } from "@vben/common-ui";
import { useI18n } from "@vben/locales";
import { formatApiError } from "@/locales/errors";
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

const { t } = useI18n();
const message = useMessage();
const isMobile = useMediaQuery("(max-width: 640px)");

const formRef = ref<FormInst | null>(null);
const loading = ref(false);
const loaded = ref(false);
const loadError = ref<unknown>(null);
const saving = ref(false);
const controlsDisabled = computed(() => loading.value || saving.value || !loaded.value);
const groups = ref<GroupView[]>([]);

const model = reactive<AdminSettings>({
  site_name: "",
  registration_enabled: false,
  guest_upload_enabled: false,
  gallery_enabled: false,
  api_enabled: false,
  trash_days: 7,
  guest_group_id: 0,
  default_group_id: 0,
});

/** 用户组选项 + 「未设置（0）」哨兵项（0 = 后端未配置该组）。 */
const groupOptions = computed(() => [
  { label: t("admin.settings.unset"), value: 0 },
  ...groups.value.map((group) => ({ label: group.name, value: group.id })),
]);

async function load(snapshot?: AdminSettings): Promise<void> {
  if (loading.value) return;
  loading.value = true;
  if (!snapshot) loaded.value = false;
  loadError.value = null;
  try {
    const settings = await getSettings();
    for (const key of Object.keys(settings) as (keyof AdminSettings)[]) {
      // Preserve any newer input event instead of overwriting it with a late refresh.
      if (!snapshot || Object.is(model[key], snapshot[key])) {
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
  void loadGroups();
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

/** 保存全量八字段（后端按 PATCH 语义处理），成功后回读刷新。 */
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
    const snapshot = { ...model, trash_days: model.trash_days ?? 0 };
    await putSettings(snapshot);
    message.success(() => t("admin.settings.saved"));
    await load(snapshot);
  } catch (error) {
    message.error(() => formatApiError(error, "admin.errors.save"));
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <Page
    :title="t('admin.settings.title')"
    :description="t('admin.settings.description')"
    class="settings-view min-w-0"
    header-class="flex-wrap items-center gap-4"
    content-class="min-w-0"
  >
    <template #extra>
      <NButton type="primary" :loading="saving" :disabled="controlsDisabled" @click="save">
        {{ t("admin.settings.save") }}
      </NButton>
    </template>
    <NCard :bordered="false">
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
        <NFormItem :label="t('admin.settings.siteName')" path="site_name">
          <div class="field">
            <NInput
              v-model:value="model.site_name"
              class="field-control"
              :placeholder="t('admin.settings.siteName')"
              :disabled="controlsDisabled"
            />
            <p class="field-hint">{{ t("admin.settings.siteNameHint") }}</p>
          </div>
        </NFormItem>
        <NFormItem :label="t('admin.settings.registration')" path="registration_enabled">
          <div class="field">
            <NSwitch v-model:value="model.registration_enabled" :disabled="controlsDisabled" />
            <p class="field-hint">{{ t("admin.settings.registrationHint") }}</p>
          </div>
        </NFormItem>
        <NFormItem :label="t('admin.settings.guestUpload')" path="guest_upload_enabled">
          <div class="field">
            <NSwitch v-model:value="model.guest_upload_enabled" :disabled="controlsDisabled" />
            <p class="field-hint">
              {{ t("admin.settings.guestUploadHint") }}
            </p>
          </div>
        </NFormItem>
        <NFormItem :label="t('admin.settings.gallery')" path="gallery_enabled">
          <div class="field">
            <NSwitch v-model:value="model.gallery_enabled" :disabled="controlsDisabled" />
            <p class="field-hint">{{ t("admin.settings.galleryHint") }}</p>
          </div>
        </NFormItem>
        <NFormItem :label="t('admin.settings.api')" path="api_enabled">
          <div class="field">
            <NSwitch v-model:value="model.api_enabled" :disabled="controlsDisabled" />
            <p class="field-hint">
              {{ t("admin.settings.apiHint") }}
            </p>
          </div>
        </NFormItem>
        <NFormItem :label="t('admin.settings.retention')" path="trash_days">
          <div class="field">
            <NInputNumber
              v-model:value="model.trash_days"
              class="field-control"
              :min="0"
              :placeholder="t('admin.settings.retention')"
              :disabled="controlsDisabled"
            />
            <p class="field-hint">{{ t("admin.settings.retentionHint") }}</p>
          </div>
        </NFormItem>
        <NFormItem :label="t('admin.settings.guestGroup')" path="guest_group_id">
          <div class="field">
            <NSelect
              v-model:value="model.guest_group_id"
              class="field-control"
              :options="groupOptions"
              :placeholder="t('admin.settings.guestGroupPlaceholder')"
              :disabled="controlsDisabled"
            />
            <p class="field-hint">{{ t("admin.settings.guestGroupHint") }}</p>
          </div>
        </NFormItem>
        <NFormItem :label="t('admin.settings.defaultGroup')" path="default_group_id">
          <div class="field">
            <NSelect
              v-model:value="model.default_group_id"
              class="field-control"
              :options="groupOptions"
              :placeholder="t('admin.settings.defaultGroupPlaceholder')"
              :disabled="controlsDisabled"
            />
            <p class="field-hint">{{ t("admin.settings.defaultGroupHint") }}</p>
          </div>
        </NFormItem>
      </NForm>
    </NCard>
    <CaptchaSettingsCard class="mt-5" />
  </Page>
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
</style>
