<script setup lang="ts">
import { useI18n } from "@vben/locales";
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NEmpty,
  NPopconfirm,
  NTag,
  useMessage,
  type DataTableColumns,
} from "naive-ui";
import { computed, h, shallowRef } from "vue";
import { revokeToken, type TokenView } from "@/api/tokens";
import { formatApiError } from "@/locales/errors";
import { useAccountScope } from "./useAccountScope";

const props = defineProps<{ tokens: TokenView[]; loading: boolean; error: string | null }>();
const emit = defineEmits<{ refresh: [] }>();
const { t, locale } = useI18n();
const message = useMessage();
const revokingId = shallowRef<number | null>(null);
const scope = useAccountScope(() => {
  revokingId.value = null;
});
const dateFormat = computed(
  () => new Intl.DateTimeFormat(locale.value, { dateStyle: "medium", timeStyle: "short" }),
);
function formatTimestamp(iso: string) {
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? iso : dateFormat.value.format(date);
}
async function revoke(row: TokenView): Promise<void> {
  if (revokingId.value !== null || !scope.available.value) return;
  revokingId.value = row.id;
  const version = scope.capture();
  try {
    await revokeToken(row.id);
    if (!scope.isCurrent(version)) return;
    message.success(() => t("account.revokeSuccess", { name: row.name }));
    emit("refresh");
  } catch (error) {
    if (scope.isCurrent(version)) message.error(() => formatApiError(error, "account.revokeError"));
  } finally {
    revokingId.value = null;
  }
}
const columns = computed<DataTableColumns<TokenView>>(() => [
  { title: t("account.name"), key: "name", width: 180, ellipsis: { tooltip: true } },
  {
    title: t("account.kind"),
    key: "kind",
    width: 90,
    render: (row) =>
      h(
        NTag,
        { size: "small", bordered: false, type: row.kind === "web" ? "info" : "success" },
        { default: () => row.kind },
      ),
  },
  {
    title: t("account.abilities"),
    key: "abilities",
    width: 110,
    render: (row) => row.abilities.join(", "),
  },
  {
    title: t("account.expiresAt"),
    key: "expires_at",
    width: 200,
    render: (row) => (row.expires_at ? formatTimestamp(row.expires_at) : t("account.neverExpires")),
  },
  {
    title: t("account.lastUsed"),
    key: "last_used_at",
    width: 200,
    render: (row) =>
      row.last_used_at ? formatTimestamp(row.last_used_at) : t("account.neverUsed"),
  },
  {
    title: t("account.createdAt"),
    key: "created_at",
    width: 200,
    render: (row) => formatTimestamp(row.created_at),
  },
  {
    title: t("account.actions"),
    key: "actions",
    width: 120,
    render: (row) =>
      h(
        NPopconfirm,
        {
          positiveText: t("account.confirmRevoke"),
          negativeText: t("account.cancel"),
          onPositiveClick: () => revoke(row),
        },
        {
          default: () => t("account.revokeWarning"),
          trigger: () =>
            h(
              NButton,
              {
                size: "tiny",
                type: "error",
                secondary: true,
                loading: revokingId.value === row.id,
                disabled: revokingId.value !== null || props.loading || !scope.available.value,
              },
              { default: () => t("account.revoke") },
            ),
        },
      ),
  },
]);
</script>
<template>
  <NCard :title="t('account.listTitle')" :bordered="false">
    <NAlert v-if="error" type="error" class="token-list__error"
      >{{ error
      }}<NButton size="tiny" quaternary :loading="loading" @click="emit('refresh')">{{
        t("account.retry")
      }}</NButton></NAlert
    >
    <NDataTable
      :columns="columns"
      :data="tokens"
      :loading="loading"
      :row-key="(row: TokenView) => row.id"
      :scroll-x="1100"
      ><template #empty><NEmpty :description="t('account.empty')" /></template
    ></NDataTable>
  </NCard>
</template>
<style scoped>
.token-list__error {
  margin-bottom: 16px;
}
</style>
