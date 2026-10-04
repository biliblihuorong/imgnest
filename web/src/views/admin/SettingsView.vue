<script setup lang="ts">
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
import { apiErrorMessage } from "@/components/admin/format";

const message = useMessage();

const formRef = ref<FormInst | null>(null);
const loading = ref(false);
const loadError = ref<string | null>(null);
const saving = ref(false);
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
  { label: "未设置（0）", value: 0 },
  ...groups.value.map((group) => ({ label: group.name, value: group.id })),
]);

async function load(): Promise<void> {
  loading.value = true;
  loadError.value = null;
  try {
    const settings = await getSettings();
    Object.assign(model, settings);
  } catch (error) {
    loadError.value = `站点设置加载失败：${apiErrorMessage(error)}`;
  } finally {
    loading.value = false;
  }
}

async function loadGroups(): Promise<void> {
  try {
    groups.value = await listGroups();
  } catch (error) {
    message.error(`用户组列表加载失败：${apiErrorMessage(error)}`);
  }
}

onMounted(() => {
  void load();
  void loadGroups();
});

const rules: FormRules = {
  site_name: [
    {
      required: true,
      trigger: ["blur", "input"],
      validator: (_rule: FormItemRule, value: string): boolean | Error =>
        value.trim().length > 0 ? true : new Error("请输入站点名称"),
    },
  ],
  trash_days: [
    {
      trigger: ["blur", "change"],
      validator: (_rule: FormItemRule, value: number | null): boolean | Error => {
        if (value === null) {
          return new Error("请设置回收站保留天数");
        }
        if (value < 0) {
          return new Error("回收站保留天数不能小于 0");
        }
        return true;
      },
    },
  ],
};

/** 保存全量八字段（后端按 PATCH 语义处理），成功后回读刷新。 */
async function save(): Promise<void> {
  if (saving.value) {
    return;
  }
  try {
    await formRef.value?.validate();
  } catch {
    // 客户端校验未通过，错误已就地展示
    return;
  }
  saving.value = true;
  try {
    await putSettings({ ...model, trash_days: model.trash_days ?? 0 });
    message.success("站点设置已保存");
    await load();
  } catch (error) {
    message.error(`保存失败：${apiErrorMessage(error)}`);
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <NCard title="站点设置" class="settings-view">
    <NAlert v-if="loadError" type="error" class="settings-alert">
      {{ loadError }}
      <NButton size="tiny" quaternary type="primary" @click="load">重试</NButton>
    </NAlert>
    <NForm
      ref="formRef"
      class="settings-form"
      :model="model"
      :rules="rules"
      label-placement="left"
      :label-width="150"
    >
      <NFormItem label="站点名称" path="site_name">
        <div class="field">
          <NInput
            v-model:value="model.site_name"
            class="field-control"
            placeholder="站点名称"
            :disabled="loading"
          />
          <p class="field-hint">显示在浏览器标题与页面头部。</p>
        </div>
      </NFormItem>
      <NFormItem label="开放注册" path="registration_enabled">
        <div class="field">
          <NSwitch v-model:value="model.registration_enabled" :disabled="loading" />
          <p class="field-hint">关闭后新用户无法自行注册，已有用户不受影响。</p>
        </div>
      </NFormItem>
      <NFormItem label="游客上传" path="guest_upload_enabled">
        <div class="field">
          <NSwitch v-model:value="model.guest_upload_enabled" :disabled="loading" />
          <p class="field-hint">
            允许未登录用户通过蓝空 v1 兼容接口上传，走游客用户组的容量与限流配置。
          </p>
        </div>
      </NFormItem>
      <NFormItem label="公共画廊" path="gallery_enabled">
        <div class="field">
          <NSwitch v-model:value="model.gallery_enabled" :disabled="loading" />
          <p class="field-hint">关闭后画廊页面不再对外展示公开图片。</p>
        </div>
      </NFormItem>
      <NFormItem label="蓝空 v1 API" path="api_enabled">
        <div class="field">
          <NSwitch v-model:value="model.api_enabled" :disabled="loading" />
          <p class="field-hint">
            /api/v1 蓝空兼容接口总开关，关闭后 v1 接口不再可用（原生 /api 接口不受影响）。
          </p>
        </div>
      </NFormItem>
      <NFormItem label="回收站保留天数" path="trash_days">
        <div class="field">
          <NInputNumber
            v-model:value="model.trash_days"
            class="field-control"
            :min="0"
            placeholder="回收站保留天数"
            :disabled="loading"
          />
          <p class="field-hint">回收站保留天数，0 为立即物理删除。</p>
        </div>
      </NFormItem>
      <NFormItem label="游客用户组" path="guest_group_id">
        <div class="field">
          <NSelect
            v-model:value="model.guest_group_id"
            class="field-control"
            :options="groupOptions"
            placeholder="选择游客用户组"
            :disabled="loading"
          />
          <p class="field-hint">游客上传使用该组的容量、限流与策略；0 表示未设置。</p>
        </div>
      </NFormItem>
      <NFormItem label="默认用户组" path="default_group_id">
        <div class="field">
          <NSelect
            v-model:value="model.default_group_id"
            class="field-control"
            :options="groupOptions"
            placeholder="选择默认用户组"
            :disabled="loading"
          />
          <p class="field-hint">新注册用户默认加入该组；0 表示未设置。</p>
        </div>
      </NFormItem>
    </NForm>
    <div class="settings-actions">
      <NButton type="primary" :loading="saving" @click="save">保存设置</NButton>
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
  color: rgba(128, 128, 128, 0.85);
}

.settings-actions {
  margin-top: 8px;
}
</style>
