<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NAlert, NButton, NEmpty, NSpin, NTabs, NTabPane } from "naive-ui";
import { onActivated, onDeactivated, onBeforeUnmount, shallowRef, watch } from "vue";
import { useRouter } from "vue-router";
import { listBillingOrders, type BillingOrder, type BillingPlan } from "@/api/billing";
import { useAuthStore } from "@/stores/auth";
import WalletSummary from "./WalletSummary.vue";
import PackageCatalog from "./PackageCatalog.vue";
import PurchaseDialog from "./PurchaseDialog.vue";
import BillingHistory from "./BillingHistory.vue";
import { clearPending, readPending, type PendingPurchase } from "./pendingPurchase";
import { useBilling } from "./useBilling";
const { t } = useI18n();
const auth = useAuthStore();
const router = useRouter();
const { overview, plans, loading, unavailable, error, load } = useBilling();
const selected = shallowRef<BillingPlan | null>(null),
  pending = shallowRef<PendingPurchase | null>(null),
  completed = shallowRef<BillingOrder | null>(null),
  revision = shallowRef(0),
  epoch = shallowRef(0),
  notice = shallowRef("");
let active = true;
let recovery: AbortController | undefined;
function recover() {
  epoch.value++;
  recovery?.abort();
  selected.value = null;
  pending.value = null;
  completed.value = null;
  notice.value = "";
  if (!active || !auth.user || !auth.token) return;
  const owner = auth.user.id;
  const old = readPending(owner);
  if (!old) return;
  pending.value = old;
  selected.value = old.plan;
  const controller = new AbortController();
  recovery = controller;
  void listBillingOrders({ request_id: old.input.request_id, page: 1, size: 1 }, controller.signal)
    .then((result) => {
      if (controller.signal.aborted) return;
      const order = result.items[0];
      if (order) {
        clearPending(owner);
        pending.value = null;
        selected.value = null;
        completed.value = { ...order, requires_login: false };
        void load();
      }
    })
    .catch(() => {
      /* Retain the original request after an uncertain result. */
    });
}
watch(() => [auth.token, auth.user?.id], recover, { immediate: true, flush: "sync" });
onDeactivated(() => {
  active = false;
  recovery?.abort();
  selected.value = null;
  pending.value = null;
  completed.value = null;
  notice.value = "";
});
onActivated(() => {
  if (!active) {
    active = true;
    recover();
  }
});
onBeforeUnmount(() => {
  active = false;
  recovery?.abort();
});
function purchased(order: BillingOrder) {
  completed.value = order;
  selected.value = null;
  pending.value = null;
  if (!order.requires_login) {
    revision.value++;
    void load();
  }
}
function refresh(message: string) {
  notice.value = message;
  selected.value = null;
  pending.value = null;
  void load();
}
async function login() {
  auth.clear();
  await router.replace({ path: "/login", query: { redirect: "/account/billing" } });
}
</script>
<template>
  <NAlert v-if="notice" type="warning" class="notice">{{ notice }}</NAlert>
  <NSpin v-if="loading" />
  <NEmpty v-else-if="unavailable" :description="t('billing.unavailable')" />
  <NAlert v-if="error" type="error"
    >{{ error }} <NButton @click="load">{{ t("billing.retry") }}</NButton></NAlert
  >
  <NAlert v-if="completed" type="success" class="notice"
    >{{ t("billing.purchaseSuccess", { id: completed.id })
    }}<template v-if="completed.requires_login"
      ><p>{{ t("billing.loginRequired") }}</p>
      <NButton data-billing-login @click="login">{{ t("billing.loginAgain") }}</NButton></template
    ></NAlert
  >
  <div v-if="overview" class="account-billing">
    <WalletSummary :account="overview" />
    <p class="hint">{{ t("billing.groupTermsHint") }}</p>
    <PackageCatalog
      :plans="plans"
      :balance="overview.wallet.balance_cents"
      :disabled="Boolean(selected) || completed?.requires_login || auth.user?.role !== 'user'"
      @buy="selected = $event"
    />
    <PurchaseDialog
      v-if="selected && auth.user"
      :key="epoch"
      :owner="auth.user.id"
      :plan="selected"
      :balance="overview.wallet.balance_cents"
      :pending="pending"
      @completed="purchased"
      @close="selected = null"
      @refresh="refresh"
    />
    <NTabs v-if="!completed?.requires_login" type="line"
      ><NTabPane name="ledger" :tab="t('billing.ledger')"
        ><BillingHistory kind="ledger" :revision="revision" /></NTabPane
      ><NTabPane name="orders" :tab="t('billing.orders')"
        ><BillingHistory kind="orders" :revision="revision" /></NTabPane
    ></NTabs>
  </div>
</template>
<style scoped>
.account-billing {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 20px;
}
.account-billing > * {
  min-width: 0;
  overflow-wrap: anywhere;
}
.hint {
  margin: 0;
  color: hsl(var(--muted-foreground));
  font-size: 13px;
  line-height: 1.6;
}
.notice {
  margin-bottom: 20px;
}
</style>
