<script setup lang="ts">
// 账户活动：最近在线、登录记录、API Key 用量与近 30 天调用趋势。
// 个人概览页与管理端用户详情共用；数据来源由调用方传入的 load 决定。
import { useI18n } from "@vben/locales";
import {
  NAlert,
  NButton,
  NDataTable,
  NEmpty,
  NSkeleton,
  NTooltip,
  type DataTableColumns,
} from "naive-ui";
import { computed, h, onMounted, shallowRef, watch } from "vue";
import type { ActivityView, LoginRecord } from "@/api/activity";
import type { TokenView } from "@/api/tokens";
import { formatDateTime } from "@/components/admin/format";
import { formatApiError } from "@/locales/errors";
import { describeUserAgent, relativeTime } from "./format";

const props = defineProps<{
  load: () => Promise<ActivityView>;
  /** 变化时重新加载（例如管理端切换到另一位用户）。 */
  reloadKey?: string | number;
}>();

const { t, locale } = useI18n();
const activity = shallowRef<ActivityView | null>(null);
const loading = shallowRef(false);
const error = shallowRef<unknown>(null);
let generation = 0;

async function reload(): Promise<void> {
  const current = ++generation;
  loading.value = true;
  error.value = null;
  try {
    const data = await props.load();
    if (current === generation) activity.value = data;
  } catch (cause) {
    if (current === generation) error.value = cause;
  } finally {
    if (current === generation) loading.value = false;
  }
}

onMounted(reload);
watch(
  () => props.reloadKey,
  () => {
    activity.value = null;
    void reload();
  },
);
defineExpose({ reload });

const number = computed(() => new Intl.NumberFormat(locale.value));
function when(iso: string | null | undefined): string {
  return iso ? formatDateTime(iso, locale.value) : t("activity.never");
}

const summary = computed(() => {
  const data = activity.value;
  if (!data) return [];
  return [
    {
      key: "lastSeen",
      value: data.last_seen_at
        ? relativeTime(data.last_seen_at, locale.value)
        : t("activity.never"),
      hint: data.last_seen_at ? when(data.last_seen_at) : t("activity.lastSeenHint"),
    },
    {
      key: "lastLogin",
      value: data.last_login ? relativeTime(data.last_login.at, locale.value) : t("activity.never"),
      hint: data.last_login
        ? `${when(data.last_login.at)} · ${data.last_login.ip || t("activity.unknownIp")}`
        : t("activity.noLogins"),
    },
    {
      key: "sessions",
      value: number.value.format(data.active_sessions),
      hint: t("activity.sessionsHint"),
    },
    {
      key: "apiRequests",
      value: number.value.format(data.api_requests),
      hint: t("activity.apiRequestsHint", { days: data.api_usage.length }),
    },
  ];
});

/** 柱高按 30 天里的最大值归一；没有调用的那天不画柱，只留基线。 */
const bars = computed(() => {
  const days = activity.value?.api_usage ?? [];
  const peak = Math.max(0, ...days.map((day) => day.requests));
  return days.map((day) => ({
    ...day,
    height: day.requests === 0 ? 0 : Math.max(4, Math.round((day.requests / peak) * 100)),
  }));
});
const peakRequests = computed(() => Math.max(0, ...bars.value.map((bar) => bar.requests)));

const tokenColumns = computed<DataTableColumns<TokenView>>(() => [
  { title: t("activity.tokenName"), key: "name", minWidth: 140, ellipsis: { tooltip: true } },
  {
    title: t("activity.useCount"),
    key: "use_count",
    width: 110,
    render: (row) => number.value.format(row.use_count ?? 0),
  },
  {
    title: t("activity.lastUsed"),
    key: "last_used_at",
    width: 170,
    render: (row) => when(row.last_used_at),
  },
  {
    title: t("activity.lastUsedIp"),
    key: "last_used_ip",
    width: 150,
    render: (row) => row.last_used_ip || "-",
  },
  {
    title: t("activity.expiresAt"),
    key: "expires_at",
    width: 170,
    render: (row) => (row.expires_at ? when(row.expires_at) : t("activity.neverExpires")),
  },
]);

const loginColumns = computed<DataTableColumns<LoginRecord>>(() => [
  { title: t("activity.loginAt"), key: "at", width: 170, render: (row) => when(row.at) },
  { title: t("activity.loginIp"), key: "ip", width: 150, render: (row) => row.ip || "-" },
  {
    title: t("activity.browser"),
    key: "user_agent",
    minWidth: 180,
    render: (row) =>
      h(
        NTooltip,
        { disabled: row.user_agent === "" },
        {
          trigger: () => h("span", describeUserAgent(row.user_agent) || "-"),
          default: () => row.user_agent,
        },
      ),
  },
]);
</script>

<template>
  <div class="activity-panel" :aria-busy="loading">
    <NAlert v-if="error" type="error" class="activity-panel__error">
      {{ formatApiError(error, "activity.loadError") }}
      <NButton size="tiny" quaternary type="primary" @click="reload">{{
        t("activity.retry")
      }}</NButton>
    </NAlert>
    <NSkeleton v-if="loading && !activity" text :repeat="4" />
    <template v-else-if="activity">
      <dl class="activity-panel__summary">
        <div
          v-for="item in summary"
          :key="item.key"
          class="activity-panel__stat"
          :data-stat="item.key"
        >
          <dt>{{ t(`activity.stats.${item.key}`) }}</dt>
          <dd class="activity-panel__value">{{ item.value }}</dd>
          <dd class="activity-panel__hint">{{ item.hint }}</dd>
        </div>
      </dl>

      <section class="activity-panel__section">
        <h3 class="activity-panel__heading">
          {{ t("activity.trendTitle", { days: bars.length }) }}
          <span class="activity-panel__peak">{{
            t("activity.peak", { count: number.format(peakRequests) })
          }}</span>
        </h3>
        <div
          class="activity-panel__chart"
          role="list"
          :aria-label="t('activity.trendTitle', { days: bars.length })"
        >
          <NTooltip v-for="bar in bars" :key="bar.day" :show-arrow="false">
            <template #trigger>
              <div
                class="activity-panel__bar-slot"
                role="listitem"
                tabindex="0"
                :aria-label="t('activity.barLabel', { day: bar.day, count: bar.requests })"
              >
                <span class="activity-panel__bar" :style="{ height: `${bar.height}%` }" />
              </div>
            </template>
            {{ t("activity.barLabel", { day: bar.day, count: number.format(bar.requests) }) }}
          </NTooltip>
        </div>
        <div class="activity-panel__axis" aria-hidden="true">
          <span>{{ bars[0]?.day.slice(5) }}</span>
          <span>{{ t("activity.today") }}</span>
        </div>
        <p class="activity-panel__note">{{ t("activity.trendNote") }}</p>
      </section>

      <section class="activity-panel__section">
        <h3 class="activity-panel__heading">{{ t("activity.tokensTitle") }}</h3>
        <NDataTable
          size="small"
          :columns="tokenColumns"
          :data="activity.tokens"
          :row-key="(row: TokenView) => row.id"
          :scroll-x="740"
        >
          <template #empty><NEmpty :description="t('activity.noTokens')" /></template>
        </NDataTable>
      </section>

      <section class="activity-panel__section">
        <h3 class="activity-panel__heading">{{ t("activity.loginsTitle") }}</h3>
        <NDataTable
          size="small"
          :columns="loginColumns"
          :data="activity.logins"
          :row-key="(row: LoginRecord) => `${row.at}-${row.ip}`"
          :scroll-x="500"
        >
          <template #empty><NEmpty :description="t('activity.noLogins')" /></template>
        </NDataTable>
      </section>
    </template>
  </div>
</template>

<style scoped>
.activity-panel {
  display: grid;
  gap: 20px;
  min-width: 0;
}

.activity-panel__error {
  margin: 0;
}

.activity-panel__summary {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin: 0;
}

.activity-panel__stat {
  min-width: 0;
  padding: 12px 14px;
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
}

.activity-panel__stat dt {
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}

.activity-panel__stat dd {
  margin: 0;
}

.activity-panel__value {
  margin-top: 4px !important;
  font-size: 20px;
  font-weight: 650;
  font-variant-numeric: tabular-nums;
}

.activity-panel__hint {
  overflow: hidden;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
  text-overflow: ellipsis;
  white-space: nowrap;
}

.activity-panel__section {
  min-width: 0;
}

.activity-panel__heading {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: baseline;
  margin: 0 0 10px;
  font-size: 14px;
  font-weight: 600;
}

.activity-panel__peak {
  font-size: 12px;
  font-weight: 400;
  color: hsl(var(--muted-foreground));
}

.activity-panel__chart {
  display: flex;
  gap: 2px;
  align-items: flex-end;
  height: 96px;
  border-bottom: 1px solid hsl(var(--border));
}

.activity-panel__bar-slot {
  display: flex;
  flex: 1 1 0;
  align-items: flex-end;
  height: 100%;
  min-width: 0;
  cursor: default;
  outline: none;
}

.activity-panel__bar-slot:hover,
.activity-panel__bar-slot:focus-visible {
  background: hsl(var(--accent));
}

.activity-panel__bar {
  display: block;
  width: 100%;
  border-radius: 4px 4px 0 0;
  background: hsl(var(--primary));
}

.activity-panel__axis {
  display: flex;
  justify-content: space-between;
  margin-top: 4px;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}

.activity-panel__note {
  margin: 6px 0 0;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}

@media (max-width: 900px) {
  .activity-panel__summary {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
