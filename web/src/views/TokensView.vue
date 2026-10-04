<script setup lang="ts">
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NDatePicker,
  NEmpty,
  NForm,
  NFormItem,
  NInput,
  NModal,
  NPopconfirm,
  NSpace,
  NTag,
  useMessage,
  type DataTableColumns,
  type FormInst,
  type FormItemRule,
  type FormRules,
} from "naive-ui";
import { h, onMounted, reactive, ref } from "vue";
import { createToken, listTokens, revokeToken } from "@/api/tokens";
import type { IssuedToken, TokenView } from "@/api/tokens";
import ChangePasswordCard from "@/components/account/ChangePasswordCard.vue";

const message = useMessage();

function errorText(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message) {
    return error.message;
  }
  return fallback;
}

/* ---------------- Token 列表 ---------------- */

const tokens = ref<TokenView[]>([]);
const loading = ref(false);
const loadError = ref<string | null>(null);
const revokingId = ref<number | null>(null);

async function loadTokens(): Promise<void> {
  loading.value = true;
  loadError.value = null;
  try {
    tokens.value = await listTokens();
  } catch (error) {
    loadError.value = errorText(error, "加载 Token 列表失败");
  } finally {
    loading.value = false;
  }
}

onMounted(() => {
  void loadTokens();
});

/** 显示为本地时间「YYYY-MM-DD HH:mm」；解析失败时原样返回。 */
function formatTimestamp(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return iso;
  }
  const pad = (value: number): string => String(value).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

async function revoke(row: TokenView): Promise<void> {
  revokingId.value = row.id;
  try {
    await revokeToken(row.id);
    message.success(`Token「${row.name}」已吊销`);
    await loadTokens();
  } catch (error) {
    message.error(errorText(error, "吊销失败"));
  } finally {
    revokingId.value = null;
  }
}

const columns: DataTableColumns<TokenView> = [
  { title: "名称", key: "name" },
  {
    title: "类型",
    key: "kind",
    render: (row) =>
      h(
        NTag,
        { size: "small", bordered: false, type: row.kind === "web" ? "info" : "success" },
        { default: () => row.kind },
      ),
  },
  { title: "权限", key: "abilities", render: (row) => row.abilities.join(", ") },
  {
    title: "过期时间",
    key: "expires_at",
    render: (row) => (row.expires_at ? formatTimestamp(row.expires_at) : "永不过期"),
  },
  {
    title: "最近使用",
    key: "last_used_at",
    render: (row) => (row.last_used_at ? formatTimestamp(row.last_used_at) : "—"),
  },
  { title: "创建时间", key: "created_at", render: (row) => formatTimestamp(row.created_at) },
  {
    title: "操作",
    key: "actions",
    render: (row) =>
      h(
        NPopconfirm,
        {
          to: false,
          positiveText: "确认吊销",
          negativeText: "取消",
          onPositiveClick: () => revoke(row),
        },
        {
          default: () => "吊销后该 Token 立即失效，且无法恢复。",
          trigger: () =>
            h(
              NButton,
              {
                size: "tiny",
                type: "error",
                secondary: true,
                loading: revokingId.value === row.id,
              },
              { default: () => "吊销" },
            ),
        },
      ),
  },
];

/* ---------------- 创建 Token ---------------- */

const createFormRef = ref<FormInst | null>(null);
const creating = ref(false);
const createModel = reactive<{ name: string; expires_at: number | null }>({
  name: "",
  expires_at: null,
});
const issued = ref<IssuedToken | null>(null);

const createRules: FormRules = {
  name: [
    {
      required: true,
      trigger: ["blur", "input"],
      validator: (_rule: FormItemRule, value: string): boolean | Error =>
        value.trim().length > 0 ? true : new Error("请输入 Token 名称"),
    },
  ],
};

async function submitCreate(): Promise<void> {
  if (creating.value) {
    return;
  }
  try {
    await createFormRef.value?.validate();
  } catch {
    // 客户端校验未通过，错误已就地展示，不发请求
    return;
  }
  creating.value = true;
  try {
    const payload: { name: string; expires_at?: string } = { name: createModel.name.trim() };
    if (createModel.expires_at !== null) {
      payload.expires_at = new Date(createModel.expires_at).toISOString();
    }
    issued.value = await createToken(payload);
    createModel.name = "";
    createModel.expires_at = null;
  } catch (error) {
    message.error(errorText(error, "创建 Token 失败"));
  } finally {
    creating.value = false;
  }
}

async function copyIssuedToken(): Promise<void> {
  if (!issued.value) {
    return;
  }
  try {
    await navigator.clipboard.writeText(issued.value.token);
    message.success("已复制到剪贴板");
  } catch {
    message.error("复制失败，请手动全选复制");
  }
}

/** 用户确认已保存后才关闭弹窗，并刷新列表展示新 Token 的元数据。 */
function closeIssuedModal(): void {
  issued.value = null;
  void loadTokens();
}
</script>

<template>
  <section class="tokens-view">
    <NCard title="API Token" class="tokens-view__card">
      <NForm
        ref="createFormRef"
        class="tokens-view__form"
        :model="createModel"
        :rules="createRules"
        label-placement="top"
        @submit.prevent="submitCreate"
      >
        <div class="tokens-view__form-row">
          <NFormItem label="名称" path="name" class="tokens-view__form-name">
            <NInput
              v-model:value="createModel.name"
              placeholder="例如：blog-client"
              :disabled="creating"
            />
          </NFormItem>
          <NFormItem label="过期时间" path="expires_at" class="tokens-view__form-expiry">
            <NDatePicker
              v-model:value="createModel.expires_at"
              type="datetime"
              clearable
              placeholder="留空则永不过期"
              :disabled="creating"
              style="width: 100%"
            />
          </NFormItem>
          <NButton attr-type="submit" type="primary" :loading="creating">创建 Token</NButton>
        </div>
        <p class="tokens-view__hint">
          <NTag size="small" :bordered="false" type="success">api</NTag>
          <span>
            web 类型 Token 是浏览器登录会话、登录时自动颁发；此处创建的 Token 类型固定为
            api，拥有全部能力（*），明文只在创建成功时显示一次。
          </span>
        </p>
      </NForm>
      <NAlert v-if="loadError" type="error" class="tokens-view__load-error">
        {{ loadError }}
        <NButton size="tiny" quaternary @click="loadTokens">重试</NButton>
      </NAlert>
      <NDataTable
        :columns="columns"
        :data="tokens"
        :loading="loading"
        :row-key="(row: TokenView) => row.id"
      >
        <template #empty>
          <NEmpty description="暂无 Token，可在上方创建用于 API 调用" />
        </template>
      </NDataTable>
    </NCard>

    <NCard title="账户安全" class="tokens-view__card">
      <ChangePasswordCard />
    </NCard>

    <NModal
      :show="issued !== null"
      preset="card"
      title="Token 创建成功"
      class="tokens-view__issued-modal"
      :mask-closable="false"
      :closable="false"
    >
      <NAlert type="warning" title="请立即保存">关闭后将无法再次查看，请立即保存。</NAlert>
      <p class="tokens-view__issued-label">Token（{{ issued?.info.name }}）</p>
      <pre class="tokens-view__issued-token">{{ issued?.token }}</pre>
      <template #footer>
        <NSpace>
          <NButton @click="copyIssuedToken">复制 Token</NButton>
          <NButton type="primary" @click="closeIssuedModal">我已保存，关闭</NButton>
        </NSpace>
      </template>
    </NModal>
  </section>
</template>

<style scoped>
.tokens-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 1080px;
  margin: 0 auto;
}

.tokens-view__form-row {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  align-items: flex-start;
}

.tokens-view__form-name {
  flex: 1 1 220px;
}

.tokens-view__form-expiry {
  flex: 1 1 260px;
}

.tokens-view__hint {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 0 16px;
  font-size: 13px;
  color: #7a7a7a;
}

.tokens-view__load-error {
  margin-bottom: 16px;
}

.tokens-view__issued-label {
  margin: 16px 0 8px;
  font-weight: 600;
}

.tokens-view__issued-token {
  margin: 0 0 16px;
  padding: 12px;
  font-family: Consolas, Menlo, "Courier New", monospace;
  background: rgba(128, 128, 128, 0.12);
  border-radius: 4px;
  white-space: pre-wrap;
  word-break: break-all;
  user-select: all;
}
</style>
