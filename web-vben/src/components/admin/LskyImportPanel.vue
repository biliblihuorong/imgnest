<script setup lang="ts">
import { useI18n } from "@vben/locales";
import {
  NAlert,
  NButton,
  NCard,
  NCollapse,
  NCollapseItem,
  NForm,
  NFormItem,
  NInput,
  NPopconfirm,
  NProgress,
  NRadio,
  NRadioGroup,
  NSelect,
  NSpace,
  NTag,
} from "naive-ui";
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from "vue";
import DocHelp from "@/components/help/DocHelp.vue";
import {
  cancelLskyImport,
  discardLskyDump,
  getLskyImport,
  listPolicies,
  listStorages,
  startLskyImport,
  uploadLskyDump,
  type LskyImportMode,
  type LskyImportPhase,
  type LskyImportStatus,
  type PolicyView,
  type StorageView,
} from "@/api/admin";
import { formatApiError } from "@/locales/errors";

const { t } = useI18n();
const status = shallowRef<LskyImportStatus | null>(null);
const policies = ref<PolicyView[]>([]);
const storages = ref<StorageView[]>([]);
const error = shallowRef<unknown>(null);
const errorKey = ref("admin.lskyImport.loadFailed");
const busy = ref(false);
const fileInput = ref<HTMLInputElement | null>(null);
const timezone = ref("Asia/Shanghai");
const mode = ref<LskyImportMode>("adopt");
const policyId = ref<number | null>(null);
const strategyId = ref(0);
const sourceUrl = ref("");
let timer: ReturnType<typeof setInterval> | undefined;
// 卸载后迟到的响应不得再写状态或重建轮询。
let disposed = false;

const PHASE_TAG: Record<LskyImportPhase, "default" | "info" | "success" | "error" | "warning"> = {
  idle: "default",
  accounts: "info",
  images: "info",
  done: "success",
  failed: "error",
  canceled: "warning",
};

const running = computed(() => status.value?.running === true);
const summary = computed(() => status.value?.summary ?? null);
const started = computed(() => !!status.value && status.value.phase !== "idle");
const percent = computed(() => {
  const current = status.value;
  if (!current || current.total === 0) return current?.phase === "done" ? 100 : 0;
  return Math.min(100, Math.round((current.processed / current.total) * 100));
});

const storageName = (id: number) => storages.value.find((s) => s.id === id)?.name ?? `#${id}`;
const policyOptions = computed(() =>
  policies.value.map((policy) => ({
    label: `${policy.name}（${t("admin.lskyImport.policyStorage", { name: storageName(policy.storage_id) })}）`,
    value: policy.id,
  })),
);
const strategyOptions = computed(() => [
  { label: t("admin.lskyImport.allStrategies"), value: 0 },
  ...(summary.value?.strategies ?? []).map((s) => ({
    label: `${s.name}（${s.url || "-"}，${s.images}）`,
    value: s.id,
  })),
]);

/** 默认选中 URL 与所选规则存储 Base URL 一致的蓝空策略。 */
function guessStrategy(): void {
  const policy = policies.value.find((p) => p.id === policyId.value);
  const storage = storages.value.find((s) => s.id === policy?.storage_id);
  const base = storage?.base_url.replace(/\/+$/, "");
  const match = summary.value?.strategies.find((s) => base && s.url === base);
  strategyId.value = match?.id ?? 0;
}
watch(policyId, guessStrategy);

function schedule(): void {
  if (timer !== undefined) {
    clearInterval(timer);
    timer = undefined;
  }
  if (running.value && !disposed) timer = setInterval(() => void refresh(), 2000);
}

function show(next: LskyImportStatus): void {
  if (disposed) return;
  status.value = next;
  if (next.options) {
    mode.value = next.options.mode;
    policyId.value = next.options.policy_id;
    strategyId.value = next.options.strategy_id;
    sourceUrl.value = next.options.source_url;
  }
  schedule();
}

async function refresh(): Promise<void> {
  try {
    show(await getLskyImport());
    error.value = null;
  } catch (err) {
    if (disposed) return;
    errorKey.value = "admin.lskyImport.loadFailed";
    error.value = err;
  }
}

async function load(): Promise<void> {
  try {
    const [nextPolicies, nextStorages] = await Promise.all([listPolicies(), listStorages()]);
    if (disposed) return;
    policies.value = nextPolicies;
    storages.value = nextStorages;
    if (policyId.value === null && nextPolicies.length > 0) policyId.value = nextPolicies[0]!.id;
  } catch (err) {
    if (disposed) return;
    errorKey.value = "admin.lskyImport.loadFailed";
    error.value = err;
  }
  await refresh();
  if (disposed) return;
  if (!status.value?.options) guessStrategy();
}

async function act(key: string, call: () => Promise<LskyImportStatus>): Promise<void> {
  if (busy.value) return;
  busy.value = true;
  try {
    show(await call());
    error.value = null;
  } catch (err) {
    if (disposed) return;
    errorKey.value = key;
    error.value = err;
  } finally {
    busy.value = false;
  }
}

function pickFile(): void {
  fileInput.value?.click();
}

async function onFile(event: Event): Promise<void> {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (!file) return;
  await act("admin.lskyImport.uploadFailed", () => uploadLskyDump(file, timezone.value));
  guessStrategy();
}

function start(): Promise<void> {
  return act("admin.lskyImport.startFailed", () =>
    startLskyImport({
      policy_id: policyId.value ?? 0,
      mode: mode.value,
      strategy_id: strategyId.value,
      source_url: mode.value === "copy" ? sourceUrl.value.trim() : "",
    }),
  );
}

onMounted(() => void load());
onBeforeUnmount(() => {
  disposed = true;
  if (timer !== undefined) clearInterval(timer);
  timer = undefined;
});
</script>

<template>
  <!-- 蓝空迁移：放在 站点设置 → 备份/导出 页的末尾。 -->
  <section class="lsky-import min-w-0" data-testid="lsky-import">
    <header class="lsky-import__header">
      <h2 class="lsky-import__title">
        {{ t("admin.lskyImport.title") }}<DocHelp topic="lskyMigration" />
      </h2>
      <p class="lsky-import__hint">{{ t("admin.lskyImport.description") }}</p>
    </header>
    <div class="lsky-import__stack">
      <NAlert v-if="error" type="error" class="lsky-import__alert">
        {{ formatApiError(error, errorKey) }}
      </NAlert>

      <NCard :title="t('admin.lskyImport.uploadTitle')" :bordered="false">
        <p class="lsky-import__hint">{{ t("admin.lskyImport.uploadHint") }}</p>
        <template v-if="summary">
          <p class="lsky-import__counts" data-lsky-summary>
            {{
              t("admin.lskyImport.counts", {
                groups: summary.groups,
                users: summary.users,
                tokens: summary.tokens,
                albums: summary.albums,
                images: summary.images,
              })
            }}
          </p>
          <ul class="lsky-import__strategies">
            <li v-for="strategy in summary.strategies" :key="strategy.id">
              <strong>{{ strategy.name }}</strong>
              <span class="lsky-import__muted">{{ strategy.url || "-" }}</span>
              <span>{{ t("admin.lskyImport.strategyImages") }} {{ strategy.images }}</span>
            </li>
          </ul>
          <NButton
            size="small"
            secondary
            :disabled="busy || running"
            data-action="discard"
            @click="act('admin.lskyImport.loadFailed', discardLskyDump)"
            >{{ t("admin.lskyImport.discard") }}</NButton
          >
        </template>
        <NForm v-else label-placement="top" class="lsky-import__form">
          <NFormItem :label="t('admin.lskyImport.timezone')">
            <NInput v-model:value="timezone" :disabled="busy" />
          </NFormItem>
          <NButton type="primary" :loading="busy" data-action="upload" @click="pickFile">{{
            busy ? t("admin.lskyImport.uploading") : t("admin.lskyImport.chooseFile")
          }}</NButton>
          <input
            ref="fileInput"
            type="file"
            accept=".sql,text/plain,application/sql"
            class="lsky-import__file"
            data-lsky-file
            @change="onFile"
          />
        </NForm>
      </NCard>

      <NCard v-if="summary" :title="t('admin.lskyImport.optionsTitle')" :bordered="false">
        <NAlert v-if="policies.length === 0" type="warning">{{
          t("admin.lskyImport.noPolicies")
        }}</NAlert>
        <NForm v-else label-placement="top" class="lsky-import__form">
          <NFormItem :label="t('admin.lskyImport.mode')">
            <NRadioGroup v-model:value="mode" :disabled="running">
              <NSpace vertical>
                <NRadio value="adopt">
                  <strong>{{ t("admin.lskyImport.modeAdopt") }}</strong>
                  <div class="lsky-import__muted">{{ t("admin.lskyImport.modeAdoptHint") }}</div>
                </NRadio>
                <NRadio value="copy">
                  <strong>{{ t("admin.lskyImport.modeCopy") }}</strong>
                  <div class="lsky-import__muted">{{ t("admin.lskyImport.modeCopyHint") }}</div>
                </NRadio>
              </NSpace>
            </NRadioGroup>
          </NFormItem>
          <NAlert v-if="mode === 'adopt'" type="warning" class="lsky-import__alert">
            {{ t("admin.lskyImport.adoptWarning") }}
          </NAlert>
          <NFormItem
            :label="t('admin.lskyImport.policy')"
            :feedback="t('admin.lskyImport.policyHint')"
          >
            <NSelect v-model:value="policyId" :options="policyOptions" :disabled="running" />
          </NFormItem>
          <NFormItem
            :label="t('admin.lskyImport.strategy')"
            :feedback="mode === 'adopt' ? t('admin.lskyImport.strategyHint') : undefined"
          >
            <NSelect v-model:value="strategyId" :options="strategyOptions" :disabled="running" />
          </NFormItem>
          <NFormItem
            v-if="mode === 'copy'"
            :label="t('admin.lskyImport.sourceUrl')"
            :feedback="t('admin.lskyImport.sourceUrlHint')"
          >
            <NInput v-model:value="sourceUrl" :disabled="running" placeholder="https://" />
          </NFormItem>
          <NSpace>
            <NPopconfirm
              :positive-text="t('admin.lskyImport.start')"
              :negative-text="t('admin.common.cancel')"
              @positive-click="start"
            >
              <template #trigger>
                <NButton
                  type="primary"
                  :loading="busy && !running"
                  :disabled="running || !policyId"
                  data-action="start"
                  >{{ t("admin.lskyImport.start") }}</NButton
                >
              </template>
              {{ t("admin.lskyImport.startConfirm") }}
            </NPopconfirm>
          </NSpace>
        </NForm>
      </NCard>

      <NCard
        v-if="status && started"
        :title="t('admin.lskyImport.progressTitle')"
        :bordered="false"
      >
        <div class="lsky-import__status" data-lsky-phase>
          <NTag :type="PHASE_TAG[status.phase]" :bordered="false">{{
            t(`admin.lskyImport.phase.${status.phase}`)
          }}</NTag>
          <span>{{
            t("admin.lskyImport.progress", {
              processed: status.processed,
              total: status.total,
              imported: status.imported,
              existing: status.existing,
              skipped: status.skipped,
              failed: status.failed,
            })
          }}</span>
          <NButton
            v-if="running"
            size="small"
            :loading="busy"
            data-action="cancel"
            @click="act('admin.lskyImport.loadFailed', cancelLskyImport)"
            >{{ t("admin.lskyImport.cancel") }}</NButton
          >
        </div>
        <NProgress
          type="line"
          :percentage="percent"
          :status="
            status.phase === 'failed' ? 'error' : status.phase === 'done' ? 'success' : 'default'
          "
          :processing="running"
        />
        <NAlert v-if="status.error" type="error" class="lsky-import__alert">{{
          status.error
        }}</NAlert>
        <p v-if="!running && status.failed > 0" class="lsky-import__muted">
          {{ t("admin.lskyImport.retryHint") }}
        </p>
        <NCollapse class="lsky-import__details">
          <NCollapseItem
            v-if="status.accounts?.length"
            :title="t('admin.lskyImport.accounts')"
            name="accounts"
          >
            <ul class="lsky-import__lines">
              <li v-for="line in status.accounts" :key="line">{{ line }}</li>
            </ul>
          </NCollapseItem>
          <NCollapseItem
            v-if="status.failures?.length"
            :title="`${t('admin.lskyImport.failures')}（${status.failed}）`"
            name="failures"
          >
            <ul class="lsky-import__lines">
              <li v-for="line in status.failures" :key="line">{{ line }}</li>
            </ul>
          </NCollapseItem>
          <NCollapseItem
            v-if="status.skips?.length"
            :title="`${t('admin.lskyImport.skips')}（${status.skipped}）`"
            name="skips"
          >
            <ul class="lsky-import__lines">
              <li v-for="line in status.skips" :key="line">{{ line }}</li>
            </ul>
          </NCollapseItem>
        </NCollapse>
      </NCard>
    </div>
  </section>
</template>

<style scoped>
.lsky-import__header {
  margin: 8px 0 12px;
}
.lsky-import__title {
  margin: 0 0 4px;
  font-size: 16px;
  font-weight: 600;
}
.lsky-import__stack {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.lsky-import__hint,
.lsky-import__muted {
  margin: 0 0 12px;
  font-size: 13px;
  color: hsl(var(--muted-foreground));
}

.lsky-import__alert {
  margin-bottom: 12px;
}

.lsky-import__form {
  max-width: 640px;
}

.lsky-import__file {
  display: none;
}

.lsky-import__counts {
  margin: 0 0 8px;
  font-weight: 600;
}

.lsky-import__strategies {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin: 0 0 12px;
  padding: 0;
  list-style: none;
  font-size: 13px;
}

.lsky-import__strategies li {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  overflow-wrap: anywhere;
}

.lsky-import__status {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  margin-bottom: 12px;
  font-size: 13px;
}

.lsky-import__details {
  margin-top: 12px;
}

.lsky-import__lines {
  margin: 0;
  padding-left: 18px;
  font-size: 13px;
  overflow-wrap: anywhere;
}
</style>
