<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NAlert, NButton, NCard, NPopconfirm, NSpace, NTag } from "naive-ui";
import { computed, onBeforeUnmount, onMounted, shallowRef } from "vue";
import DocHelp from "@/components/help/DocHelp.vue";
import {
  controlVariantBackfill,
  getVariantBackfill,
  type VariantBackfill,
  type VariantBackfillStatus,
} from "@/api/admin";
import { formatApiError } from "@/locales/errors";

const { t } = useI18n();
const state = shallowRef<VariantBackfill | null>(null);
const error = shallowRef<unknown>(null);
const busy = shallowRef(false);
let timer: ReturnType<typeof setInterval> | undefined;

const STATUS_TAG: Record<VariantBackfillStatus, "default" | "info" | "warning" | "success"> = {
  idle: "default",
  running: "info",
  paused: "warning",
  done: "success",
};

const running = computed(() => state.value?.status === "running");

function schedule(): void {
  if (timer !== undefined) {
    clearInterval(timer);
    timer = undefined;
  }
  if (running.value) {
    timer = setInterval(() => void load(), 3000);
  }
}

async function load(): Promise<void> {
  try {
    state.value = await getVariantBackfill();
    error.value = null;
  } catch (err) {
    error.value = err;
  }
  schedule();
}

async function control(action: "start" | "pause", restart = false): Promise<void> {
  if (busy.value) {
    return;
  }
  busy.value = true;
  try {
    state.value = await controlVariantBackfill(action, restart);
    error.value = null;
  } catch (err) {
    error.value = err;
  } finally {
    busy.value = false;
  }
  schedule();
}

onMounted(() => void load());
onBeforeUnmount(() => {
  if (timer !== undefined) {
    clearInterval(timer);
  }
});
</script>

<template>
  <NCard class="variant-backfill">
    <template #header
      >{{ t("admin.variants.title") }}<DocHelp topic="responsiveVariants"
    /></template>
    <p class="variant-backfill__intro">{{ t("admin.variants.intro") }}</p>
    <NAlert v-if="error" type="error" class="variant-backfill__notice">
      {{ formatApiError(error, "admin.variants.loadFailed") }}
    </NAlert>
    <template v-if="state">
      <div class="variant-backfill__status" data-variant-status>
        <NTag :type="STATUS_TAG[state.status]" :bordered="false">{{
          t(`admin.variants.status.${state.status}`)
        }}</NTag>
        <span>{{
          t("admin.variants.progress", {
            processed: state.processed,
            generated: state.generated,
            skipped: state.skipped,
            failed: state.failed,
          })
        }}</span>
      </div>
      <NSpace>
        <NButton
          v-if="running"
          size="small"
          :loading="busy"
          data-action="pause"
          @click="control('pause')"
          >{{ t("admin.variants.pause") }}</NButton
        >
        <NButton
          v-else
          size="small"
          type="primary"
          :loading="busy"
          data-action="start"
          @click="control('start')"
          >{{
            state.status === "paused" ? t("admin.variants.resume") : t("admin.variants.start")
          }}</NButton
        >
        <NPopconfirm
          :positive-text="t('admin.variants.restart')"
          :negative-text="t('admin.common.cancel')"
          @positive-click="control('start', true)"
        >
          <template #trigger>
            <NButton size="small" secondary :disabled="busy" data-action="restart">{{
              t("admin.variants.restart")
            }}</NButton>
          </template>
          {{ t("admin.variants.restartConfirm") }}
        </NPopconfirm>
      </NSpace>
    </template>
  </NCard>
</template>

<style scoped>
.variant-backfill__intro {
  margin: 0 0 12px;
  font-size: 13px;
  color: hsl(var(--muted-foreground));
}

.variant-backfill__notice {
  margin-bottom: 12px;
}

.variant-backfill__status {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  margin-bottom: 12px;
  font-size: 13px;
}
</style>
