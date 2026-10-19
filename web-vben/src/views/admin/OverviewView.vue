<script setup lang="ts">
// 管理端站点概览：账户/图片/存储总量、近 30 天上传、按存储与规则的用量、用量前十的用户。
// 数据一次取自 GET /api/admin/overview；字节均为云端计费字节。
import { Page } from "@vben/common-ui";
import { useI18n } from "@vben/locales";
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NEmpty,
  NSkeleton,
  NTag,
  NTooltip,
  type DataTableColumns,
} from "naive-ui";
import { computed, h, onMounted, shallowRef } from "vue";
import {
  fetchAdminOverview,
  type AdminOverview,
  type AdminPolicyUsage,
  type AdminStorageUsage,
  type AdminUserUsage,
} from "@/api/overview";
import { formatDateTime } from "@/components/admin/format";
import { formatBytes } from "@/lib/format";
import { formatApiError } from "@/locales/errors";

const { t, locale } = useI18n();
const overview = shallowRef<AdminOverview | null>(null);
const loading = shallowRef(false);
const error = shallowRef<unknown>(null);
let generation = 0;

async function reload(): Promise<void> {
  const current = ++generation;
  loading.value = true;
  error.value = null;
  try {
    const data = await fetchAdminOverview();
    if (current === generation) overview.value = data;
  } catch (cause) {
    if (current === generation) error.value = cause;
  } finally {
    if (current === generation) loading.value = false;
  }
}

onMounted(reload);

const number = computed(() => new Intl.NumberFormat(locale.value));
const count = (value: number): string => number.value.format(value);

/** 占比（百分比，保留一位）；分母为 0 时为 0。 */
function share(part: number, whole: number): number {
  return whole > 0 ? Math.round((part / whole) * 1000) / 10 : 0;
}

const stats = computed(() => {
  const data = overview.value;
  if (!data) return [];
  const totals = data.totals;
  const variants = data.variants;
  return [
    { key: "users", value: count(totals.users), hint: t("admin.overview.hints.users") },
    {
      key: "images",
      value: count(totals.images),
      hint:
        totals.pending_images > 0
          ? t("admin.overview.hints.pending", { count: count(totals.pending_images) })
          : t("admin.overview.hints.guest", {
              count: count(totals.guest_images),
              size: formatBytes(totals.guest_bytes),
            }),
    },
    {
      key: "bytes",
      value: formatBytes(totals.bytes),
      hint: t("admin.overview.hints.guest", {
        count: count(totals.guest_images),
        size: formatBytes(totals.guest_bytes),
      }),
    },
    { key: "albums", value: count(totals.albums), hint: t("admin.overview.hints.albums") },
    {
      key: "trash",
      value: formatBytes(totals.trash_bytes),
      hint: t("admin.overview.hints.trash", {
        count: count(totals.trash_images),
        size: formatBytes(totals.trash_bytes),
      }),
    },
    {
      key: "variants",
      value: variants.eligible > 0 ? `${share(variants.covered, variants.eligible)}%` : "-",
      hint:
        variants.eligible > 0
          ? t("admin.overview.hints.variants", {
              covered: count(variants.covered),
              eligible: count(variants.eligible),
            })
          : t("admin.overview.hints.variantsNone"),
    },
  ];
});

/** 柱高按 30 天里上传次数的最大值归一；没有上传的那天只留基线。 */
const bars = computed(() => {
  const days = overview.value?.uploads ?? [];
  const peak = Math.max(0, ...days.map((day) => day.uploads));
  return days.map((day) => ({
    ...day,
    height: day.uploads === 0 ? 0 : Math.max(4, Math.round((day.uploads / peak) * 100)),
    label: t("admin.overview.barLabel", {
      day: day.day,
      count: count(day.uploads),
      size: formatBytes(day.bytes),
    }),
  }));
});
const uploadTotals = computed(() =>
  bars.value.reduce(
    (sum, day) => ({ uploads: sum.uploads + day.uploads, bytes: sum.bytes + day.bytes }),
    { uploads: 0, bytes: 0 },
  ),
);

type Part = "original" | "webp" | "thumb" | "variant";
const PARTS: Part[] = ["original", "webp", "thumb", "variant"];
function partBytes(row: AdminStorageUsage, part: Part): number {
  switch (part) {
    case "original":
      return row.original_bytes;
    case "webp":
      return row.webp_bytes;
    case "thumb":
      return row.thumb_bytes;
    case "variant":
      return row.variant_bytes;
  }
}

function nameCell(name: string, enabled: boolean) {
  return h("span", { class: "overview-view__name" }, [
    h("span", name),
    enabled
      ? null
      : h(NTag, { size: "small", bordered: false }, () => t("admin.overview.disabled")),
  ]);
}

function shareCell(part: number, whole: number) {
  const percent = share(part, whole);
  return h("div", { class: "overview-view__share" }, [
    h("span", { class: "overview-view__meter", "aria-hidden": "true" }, [
      h("span", { class: "overview-view__meter-fill", style: { width: `${percent}%` } }),
    ]),
    h("span", { class: "overview-view__share-text" }, `${percent}%`),
  ]);
}

function breakdownCell(row: AdminStorageUsage) {
  if (row.bytes <= 0) return h("span", "-");
  const label = PARTS.map(
    (part) => `${t(`admin.overview.parts.${part}`)} ${formatBytes(partBytes(row, part))}`,
  ).join(" · ");
  return h(
    NTooltip,
    { showArrow: false },
    {
      trigger: () =>
        h(
          "span",
          { class: "overview-view__stack", role: "img", "aria-label": label, tabindex: 0 },
          PARTS.map((part) =>
            h("span", {
              class: `overview-view__stack-part overview-view__stack-part--${part}`,
              style: { width: `${share(partBytes(row, part), row.bytes)}%` },
            }),
          ),
        ),
      default: () => label,
    },
  );
}

const storageColumns = computed<DataTableColumns<AdminStorageUsage>>(() => [
  {
    title: t("admin.overview.storage"),
    key: "name",
    minWidth: 150,
    render: (row) => nameCell(row.name, row.enabled),
  },
  { title: t("admin.overview.driver"), key: "driver", width: 80 },
  {
    title: t("admin.overview.imageCount"),
    key: "images",
    width: 100,
    align: "right",
    render: (row) => count(row.images),
  },
  {
    title: t("admin.overview.bytes"),
    key: "bytes",
    width: 110,
    align: "right",
    render: (row) => formatBytes(row.bytes),
  },
  {
    title: t("admin.overview.share"),
    key: "share",
    width: 150,
    render: (row) => shareCell(row.bytes, overview.value?.totals.bytes ?? 0),
  },
  {
    title: t("admin.overview.breakdown"),
    key: "breakdown",
    minWidth: 160,
    render: breakdownCell,
  },
  {
    title: t("admin.overview.trash"),
    key: "trash_bytes",
    width: 130,
    align: "right",
    render: (row) =>
      row.trash_images > 0 ? `${formatBytes(row.trash_bytes)} (${count(row.trash_images)})` : "-",
  },
]);

const storageNames = computed(
  () => new Map((overview.value?.storages ?? []).map((storage) => [storage.id, storage.name])),
);

const policyColumns = computed<DataTableColumns<AdminPolicyUsage>>(() => [
  {
    title: t("admin.overview.policy"),
    key: "name",
    minWidth: 150,
    render: (row) => nameCell(row.name, row.enabled),
  },
  {
    title: t("admin.overview.storage"),
    key: "storage_id",
    minWidth: 120,
    render: (row) => storageNames.value.get(row.storage_id) ?? `#${row.storage_id}`,
  },
  {
    title: t("admin.overview.imageCount"),
    key: "images",
    width: 100,
    align: "right",
    render: (row) => count(row.images),
  },
  {
    title: t("admin.overview.bytes"),
    key: "bytes",
    width: 110,
    align: "right",
    render: (row) => formatBytes(row.bytes),
  },
  {
    title: t("admin.overview.share"),
    key: "share",
    width: 150,
    render: (row) => shareCell(row.bytes, overview.value?.totals.bytes ?? 0),
  },
]);

const userColumns = computed<DataTableColumns<AdminUserUsage>>(() => [
  { title: "#", key: "rank", width: 48, render: (_row, index) => index + 1 },
  {
    title: t("admin.overview.username"),
    key: "username",
    minWidth: 140,
    ellipsis: { tooltip: true },
  },
  {
    title: t("admin.overview.imageCount"),
    key: "images",
    width: 100,
    align: "right",
    render: (row) => count(row.images),
  },
  {
    title: t("admin.overview.usedBytes"),
    key: "used_bytes",
    width: 110,
    align: "right",
    render: (row) => formatBytes(row.used_bytes),
  },
  {
    title: t("admin.overview.share"),
    key: "share",
    width: 150,
    render: (row) => shareCell(row.used_bytes, overview.value?.totals.bytes ?? 0),
  },
]);
</script>

<template>
  <Page
    class="overview-view"
    :title="t('admin.overview.title')"
    :description="t('admin.overview.description')"
    header-class="flex-wrap gap-4"
  >
    <template #extra>
      <NButton :loading="loading" data-testid="overview-refresh" @click="reload">{{
        t("admin.overview.refresh")
      }}</NButton>
    </template>
    <div class="overview-view__content" :aria-busy="loading">
      <NAlert v-if="error" type="error" data-testid="overview-error">
        {{ formatApiError(error, "admin.overview.loadError") }}
        <NButton size="tiny" quaternary type="primary" @click="reload">{{
          t("admin.overview.retry")
        }}</NButton>
      </NAlert>
      <NSkeleton v-if="loading && !overview" text :repeat="6" />
      <template v-else-if="overview">
        <dl class="overview-view__stats">
          <div
            v-for="item in stats"
            :key="item.key"
            class="overview-view__stat"
            :data-stat="item.key"
          >
            <dt>{{ t(`admin.overview.stats.${item.key}`) }}</dt>
            <dd class="overview-view__value">{{ item.value }}</dd>
            <dd class="overview-view__hint">{{ item.hint }}</dd>
          </div>
        </dl>

        <NCard :bordered="false" data-testid="overview-uploads">
          <template #header>
            <h2 class="overview-view__heading">
              {{ t("admin.overview.uploadsTitle", { days: bars.length }) }}
              <span class="overview-view__muted">{{
                t("admin.overview.uploadsTotal", {
                  count: count(uploadTotals.uploads),
                  size: formatBytes(uploadTotals.bytes),
                })
              }}</span>
            </h2>
          </template>
          <div
            class="overview-view__chart"
            role="list"
            :aria-label="t('admin.overview.uploadsTitle', { days: bars.length })"
          >
            <NTooltip v-for="bar in bars" :key="bar.day" :show-arrow="false">
              <template #trigger>
                <div
                  class="overview-view__bar-slot"
                  role="listitem"
                  tabindex="0"
                  :aria-label="bar.label"
                >
                  <span class="overview-view__bar" :style="{ height: `${bar.height}%` }" />
                </div>
              </template>
              {{ bar.label }}
            </NTooltip>
          </div>
          <div class="overview-view__axis" aria-hidden="true">
            <span>{{ bars[0]?.day.slice(5) }}</span>
            <span>{{ t("admin.overview.today") }}</span>
          </div>
          <p class="overview-view__note">{{ t("admin.overview.uploadsNote") }}</p>
        </NCard>

        <NCard :bordered="false" data-testid="overview-storages">
          <template #header>
            <h2 class="overview-view__heading">{{ t("admin.overview.storagesTitle") }}</h2>
          </template>
          <ul class="overview-view__legend">
            <li v-for="part in PARTS" :key="part">
              <span :class="`overview-view__swatch overview-view__stack-part--${part}`" />
              {{ t(`admin.overview.parts.${part}`) }}
            </li>
          </ul>
          <NDataTable
            size="small"
            :columns="storageColumns"
            :data="overview.storages"
            :row-key="(row: AdminStorageUsage) => row.id"
            :scroll-x="880"
          >
            <template #empty><NEmpty :description="t('admin.overview.empty')" /></template>
          </NDataTable>
        </NCard>

        <div class="overview-view__pair">
          <NCard :bordered="false" data-testid="overview-policies">
            <template #header>
              <h2 class="overview-view__heading">{{ t("admin.overview.policiesTitle") }}</h2>
            </template>
            <NDataTable
              size="small"
              :columns="policyColumns"
              :data="overview.policies"
              :row-key="(row: AdminPolicyUsage) => row.id"
              :scroll-x="640"
            >
              <template #empty><NEmpty :description="t('admin.overview.empty')" /></template>
            </NDataTable>
          </NCard>
          <NCard :bordered="false" data-testid="overview-users">
            <template #header>
              <h2 class="overview-view__heading">
                {{ t("admin.overview.topUsersTitle", { count: overview.top_users.length || 10 }) }}
              </h2>
            </template>
            <NDataTable
              size="small"
              :columns="userColumns"
              :data="overview.top_users"
              :row-key="(row: AdminUserUsage) => row.id"
              :scroll-x="560"
            >
              <template #empty><NEmpty :description="t('admin.overview.empty')" /></template>
            </NDataTable>
          </NCard>
        </div>
        <p class="overview-view__note">
          {{
            t("admin.overview.generatedAt", { time: formatDateTime(overview.generated_at, locale) })
          }}
        </p>
      </template>
    </div>
  </Page>
</template>

<style scoped>
.overview-view__content {
  display: grid;
  gap: 16px;
  min-width: 0;
}

.overview-view__stats {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 12px;
  margin: 0;
}

.overview-view__stat {
  min-width: 0;
  padding: 12px 14px;
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
  background: hsl(var(--card));
}

.overview-view__stat dt {
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}

.overview-view__stat dd {
  margin: 0;
}

.overview-view__value {
  margin-top: 4px !important;
  font-size: 20px;
  font-weight: 650;
  font-variant-numeric: tabular-nums;
}

.overview-view__hint {
  overflow: hidden;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
  text-overflow: ellipsis;
  white-space: nowrap;
}

.overview-view__heading {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: baseline;
  margin: 0;
  font-size: 14px;
  font-weight: 600;
}

.overview-view__muted {
  font-size: 12px;
  font-weight: 400;
  color: hsl(var(--muted-foreground));
}

.overview-view__chart {
  display: flex;
  gap: 2px;
  align-items: flex-end;
  height: 120px;
  border-bottom: 1px solid hsl(var(--border));
}

.overview-view__bar-slot {
  display: flex;
  flex: 1 1 0;
  align-items: flex-end;
  height: 100%;
  min-width: 0;
  cursor: default;
  outline: none;
}

.overview-view__bar-slot:hover,
.overview-view__bar-slot:focus-visible {
  background: hsl(var(--accent));
}

.overview-view__bar {
  display: block;
  width: 100%;
  border-radius: 4px 4px 0 0;
  background: hsl(var(--primary));
}

.overview-view__axis {
  display: flex;
  justify-content: space-between;
  margin-top: 4px;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}

.overview-view__note {
  margin: 6px 0 0;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}

.overview-view__pair {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
  min-width: 0;
}

.overview-view__name {
  display: inline-flex;
  gap: 6px;
  align-items: center;
}

.overview-view__share {
  display: flex;
  gap: 8px;
  align-items: center;
}

.overview-view__meter {
  flex: 1 1 auto;
  height: 6px;
  overflow: hidden;
  border-radius: 3px;
  background: hsl(var(--accent));
}

.overview-view__meter-fill {
  display: block;
  height: 100%;
  background: hsl(var(--primary));
}

.overview-view__share-text {
  min-width: 44px;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  text-align: right;
}

.overview-view__stack {
  display: flex;
  height: 10px;
  overflow: hidden;
  border-radius: 3px;
  background: hsl(var(--accent));
  outline: none;
}

.overview-view__stack-part--original {
  background: hsl(var(--primary));
}

.overview-view__stack-part--webp {
  background: hsl(var(--primary) / 60%);
}

.overview-view__stack-part--thumb {
  background: hsl(var(--primary) / 35%);
}

.overview-view__stack-part--variant {
  background: hsl(var(--primary) / 18%);
}

.overview-view__legend {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin: 0 0 10px;
  padding: 0;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
  list-style: none;
}

.overview-view__legend li {
  display: inline-flex;
  gap: 6px;
  align-items: center;
}

.overview-view__swatch {
  display: inline-block;
  width: 10px;
  height: 10px;
  border-radius: 2px;
}

@media (max-width: 1200px) {
  .overview-view__stats {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .overview-view__pair {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (max-width: 560px) {
  .overview-view__stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
