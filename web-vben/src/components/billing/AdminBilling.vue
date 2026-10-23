<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NAlert, NButton, NEmpty, NSpin, NTabs, NTabPane } from "naive-ui";
import { computed, onActivated, onBeforeUnmount, onDeactivated, shallowRef, watch } from "vue";
import { useRoute } from "vue-router";
import { ApiError } from "@/api/client";
import { listAdminBillingPlans, type BillingPlan, type BillingEntry } from "@/api/billing";
import { listGroups, type GroupView } from "@/api/admin";
import { useAuthStore } from "@/stores/auth";
import { formatApiError } from "@/locales/errors";
import AdminPackageList from "./AdminPackageList.vue";
import BillingHistory from "./BillingHistory.vue";
import PlanEditor from "./PlanEditor.vue";
import WalletAdjustment from "./WalletAdjustment.vue";
const { t } = useI18n();
const auth = useAuthStore();
const route = useRoute();
const plans = shallowRef<BillingPlan[]>([]),
  groups = shallowRef<GroupView[]>([]),
  loading = shallowRef(false),
  error = shallowRef(""),
  unavailable = shallowRef(false),
  active = shallowRef(true),
  editorOpen = shallowRef(false),
  editing = shallowRef<BillingPlan | null>(null),
  reversal = shallowRef<BillingEntry | null>(null),
  revision = shallowRef(0);
const userInput = shallowRef(typeof route.query.user_id === "string" ? route.query.user_id : ""),
  from = shallowRef(""),
  to = shallowRef(""),
  userId = shallowRef<number>(),
  filterFrom = shallowRef<string>(),
  filterTo = shallowRef<string>();
let request: AbortController | undefined;
function clear() {
  request?.abort();
  plans.value = [];
  groups.value = [];
  editorOpen.value = false;
  editing.value = null;
  reversal.value = null;
  loading.value = false;
  error.value = "";
  unavailable.value = false;
}
async function load() {
  clear();
  if (!active.value || !auth.token || auth.user?.role !== "admin") return;
  const controller = new AbortController();
  request = controller;
  loading.value = true;
  try {
    const [offers, availableGroups] = await Promise.all([
      listAdminBillingPlans(controller.signal),
      listGroups(),
    ]);
    if (controller.signal.aborted || request !== controller) return;
    plans.value = offers;
    groups.value = availableGroups;
  } catch (cause) {
    if (controller.signal.aborted) return;
    if (cause instanceof ApiError && cause.status === 404) unavailable.value = true;
    else error.value = formatApiError(cause, "billing.loadError");
  } finally {
    if (request === controller && !controller.signal.aborted) loading.value = false;
  }
}
watch(
  () => [auth.token, auth.user?.id, auth.user?.role],
  () => {
    userInput.value = "";
    userId.value = undefined;
    from.value = "";
    to.value = "";
    filterFrom.value = undefined;
    filterTo.value = undefined;
    void load();
  },
  { immediate: true, flush: "sync" },
);
onDeactivated(() => {
  active.value = false;
  clear();
});
onActivated(() => {
  if (!active.value) {
    active.value = true;
    void load();
  }
});
onBeforeUnmount(() => {
  active.value = false;
  clear();
});
const validFilter = computed(
  () =>
    (!userInput.value ||
      (/^[1-9]\d*$/.test(userInput.value) && Number.isSafeInteger(Number(userInput.value)))) &&
    (!from.value || Number.isFinite(Date.parse(from.value + "Z"))) &&
    (!to.value || Number.isFinite(Date.parse(to.value + "Z"))) &&
    (!from.value || !to.value || from.value < to.value),
);
function filter() {
  if (!validFilter.value) return;
  userId.value = userInput.value ? Number(userInput.value) : undefined;
  filterFrom.value = from.value ? new Date(from.value + "Z").toISOString() : undefined;
  filterTo.value = to.value ? new Date(to.value + "Z").toISOString() : undefined;
  revision.value++;
}
function edit(plan: BillingPlan | null) {
  editing.value = plan;
  editorOpen.value = true;
}
</script>
<template>
  <div v-if="active" class="admin-billing">
    <NSpin v-if="loading" /><NEmpty
      v-else-if="unavailable"
      :description="t('billing.unavailable')"
    />
    <NAlert v-if="error" type="error"
      >{{ error }} <NButton @click="load">{{ t("billing.retry") }}</NButton></NAlert
    >
    <template v-if="!unavailable && !loading && !error && auth.user?.role === 'admin'">
      <div class="toolbar">
        <NButton type="primary" @click="edit(null)">{{ t("billing.newPlan") }}</NButton
        ><NButton @click="load">{{ t("billing.refresh") }}</NButton>
      </div>
      <PlanEditor
        v-if="editorOpen"
        :key="editing ? `${editing.id}:${editing.version}` : 'new'"
        :plan="editing"
        :groups="groups"
        @close="editorOpen = false"
        @saved="load"
      />
      <AdminPackageList :plans="plans" @edit="edit" />
      <form class="filters" @submit.prevent="filter">
        <label>{{ t("billing.filterUser") }}<input v-model="userInput" inputmode="numeric" /></label
        ><label>{{ t("billing.from") }}<input v-model="from" type="datetime-local" /></label
        ><label>{{ t("billing.to") }}<input v-model="to" type="datetime-local" /></label
        ><NButton attr-type="submit" :disabled="!validFilter">{{ t("billing.filter") }}</NButton>
      </form>
      <WalletAdjustment
        :key="auth.user.id"
        :user-id="userId"
        :reverse="reversal"
        @saved="
          revision++;
          reversal = null;
        "
      />
      <NTabs type="line"
        ><NTabPane name="wallets" :tab="t('billing.wallets')"
          ><BillingHistory
            kind="wallets"
            admin
            :user-id="userId"
            :from="filterFrom"
            :to="filterTo"
            :revision="revision" /></NTabPane
        ><NTabPane name="ledger" :tab="t('billing.ledger')"
          ><BillingHistory
            kind="ledger"
            admin
            :user-id="userId"
            :from="filterFrom"
            :to="filterTo"
            :revision="revision"
            @reverse="reversal = $event" /></NTabPane
        ><NTabPane name="orders" :tab="t('billing.orders')"
          ><BillingHistory
            kind="orders"
            admin
            :user-id="userId"
            :from="filterFrom"
            :to="filterTo"
            :revision="revision" /></NTabPane
        ><NTabPane name="memberships" :tab="t('billing.memberships')"
          ><BillingHistory
            kind="memberships"
            admin
            :user-id="userId"
            :from="filterFrom"
            :to="filterTo"
            :revision="revision" /></NTabPane
      ></NTabs>
    </template>
  </div>
</template>
<style scoped>
.admin-billing {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 20px;
}
.admin-billing > * {
  min-width: 0;
  overflow-wrap: anywhere;
}
.toolbar,
.filters {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: end;
}
.filters label {
  display: grid;
  gap: 6px;
  flex: 1 1 180px;
}
.filters input {
  padding: 9px 12px;
  background: hsl(var(--background));
  border: 1px solid hsl(var(--border));
  border-radius: var(--radius);
  color: inherit;
  min-width: 0;
}
</style>
