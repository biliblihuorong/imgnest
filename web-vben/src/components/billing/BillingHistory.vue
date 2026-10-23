<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NAlert, NButton, NCard, NDataTable, NPagination, type DataTableColumns } from "naive-ui";
import { computed, h, onActivated, onBeforeUnmount, onDeactivated, shallowRef, watch } from "vue";
import * as api from "@/api/billing";
import { useAuthStore } from "@/stores/auth";
import { formatApiError } from "@/locales/errors";
import { formatDateTime } from "@/components/admin/format";
import { formatCents } from "./money";
const props = defineProps<{
  kind: "ledger" | "orders" | "memberships" | "wallets";
  admin?: boolean;
  userId?: number;
  revision?: number;
  from?: string;
  to?: string;
}>();
const emit = defineEmits<{ reverse: [entry: api.BillingEntry] }>();
const { t } = useI18n();
const auth = useAuthStore();
type Row = api.BillingEntry | api.BillingOrder | api.BillingMembership | api.BillingWallet;
const rows = shallowRef<Row[]>([]),
  page = shallowRef(1),
  total = shallowRef(0),
  loading = shallowRef(false),
  error = shallowRef("");
let active = true,
  request: AbortController | undefined;
function clear() {
  request?.abort();
  rows.value = [];
  total.value = 0;
  error.value = "";
  loading.value = false;
}
async function load() {
  clear();
  if (!active || !auth.token || !auth.user) return;
  const controller = new AbortController();
  request = controller;
  loading.value = true;
  const params = {
    page: page.value,
    size: 20,
    ...(props.admin && props.userId ? { user_id: props.userId } : {}),
    ...(props.from ? { from: props.from } : {}),
    ...(props.to ? { to: props.to } : {}),
  };
  try {
    const loader =
      props.kind === "wallets"
        ? api.listBillingWallets
        : props.kind === "memberships"
          ? api.listBillingMemberships
          : props.kind === "ledger"
            ? props.admin
              ? api.listAdminBillingLedger
              : api.listBillingLedger
            : props.admin
              ? api.listAdminBillingOrders
              : api.listBillingOrders;
    const result = await loader(params, controller.signal);
    if (controller.signal.aborted || request !== controller) return;
    rows.value = result.items;
    total.value = result.total;
  } catch (cause) {
    if (!controller.signal.aborted) error.value = formatApiError(cause, "billing.loadError");
  } finally {
    if (request === controller && !controller.signal.aborted) loading.value = false;
  }
}
watch(
  () => [auth.token, auth.user?.id, props.kind, props.userId, props.revision, props.from, props.to],
  () => {
    page.value = 1;
    void load();
  },
  { immediate: true, flush: "sync" },
);
watch(page, () => void load());
onDeactivated(() => {
  active = false;
  clear();
});
onActivated(() => {
  if (!active) {
    active = true;
    void load();
  }
});
onBeforeUnmount(() => {
  active = false;
  clear();
});
const columns = computed<DataTableColumns<Row>>(() => {
  const shared: DataTableColumns<Row> = [
    {
      title: t("billing.id"),
      key: "id",
      render: (row) => ("id" in row ? String(row.id) : String(row.user_id)),
    },
    {
      title: t("billing.user"),
      key: "user_id",
      render: (row) =>
        "username" in row ? `${row.username} (#${row.user_id})` : `#${row.user_id}`,
    },
  ];
  if (props.kind === "ledger") {
    shared.push(
      {
        title: t("billing.kind"),
        key: "kind",
        render: (row) => ("kind" in row ? t(`billing.kinds.${row.kind}`) : ""),
      },
      {
        title: t("billing.amount"),
        key: "amount_cents",
        render: (row) => ("amount_cents" in row ? `¥${formatCents(row.amount_cents)}` : ""),
      },
      {
        title: t("billing.balanceAfter"),
        key: "after_cents",
        render: (row) => ("after_cents" in row ? `¥${formatCents(row.after_cents)}` : ""),
      },
      {
        title: t("billing.reason"),
        key: "reason",
        render: (row) => ("reason" in row ? row.reason : ""),
      },
    );
    if (props.admin)
      shared.push(
        {
          title: t("billing.actor"),
          key: "actor_name",
          render: (row) => ("actor_name" in row ? `${row.actor_name} (#${row.actor_id})` : ""),
        },
        {
          title: t("billing.reversal"),
          key: "reverse",
          render: (row) =>
            "kind" in row && ["admin_credit", "admin_debit"].includes(row.kind)
              ? h(NButton, { size: "small", onClick: () => emit("reverse", row) }, () =>
                  t("billing.reversal"),
                )
              : null,
        },
      );
  } else if (props.kind === "wallets")
    shared.push({
      title: t("billing.balance"),
      key: "balance_cents",
      render: (row) => ("balance_cents" in row ? `¥${formatCents(row.balance_cents)}` : ""),
    });
  else if (props.kind === "orders")
    shared.push(
      { title: t("billing.package"), key: "plan_name" },
      {
        title: t("billing.amount"),
        key: "price_cents",
        render: (row) => ("price_cents" in row ? `¥${formatCents(row.price_cents)}` : ""),
      },
      {
        title: t("billing.expiresAt"),
        key: "expires_at",
        render: (row) => ("expires_at" in row ? formatDateTime(row.expires_at) : ""),
      },
    );
  else
    shared.push(
      { title: t("billing.package"), key: "plan_id" },
      {
        title: t("billing.state"),
        key: "status",
        render: (row) => ("status" in row ? t(`billing.states.${row.status}`) : ""),
      },
      {
        title: t("billing.expiresAt"),
        key: "expires_at",
        render: (row) => ("expires_at" in row ? formatDateTime(row.expires_at) : ""),
      },
    );
  shared.push({
    title: t("billing.time"),
    key: "created_at",
    render: (row) => formatDateTime("created_at" in row ? row.created_at : row.updated_at),
  });
  return shared;
});
</script>
<template>
  <NCard :title="t(`billing.${kind}`)" :bordered="false"
    ><NAlert v-if="error" type="error"
      >{{ error }} <NButton size="small" @click="load">{{ t("billing.retry") }}</NButton></NAlert
    ><NDataTable
      :columns="columns"
      :data="rows"
      :loading="loading"
      :scroll-x="admin ? 1050 : 720" /><NPagination
      v-if="total > 20"
      v-model:page="page"
      :item-count="total"
      :page-size="20"
      class="pagination"
  /></NCard>
</template>
<style scoped>
.pagination {
  margin-top: 16px;
  justify-content: flex-end;
}
</style>
