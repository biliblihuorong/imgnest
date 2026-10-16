<script setup lang="ts">
// 备份与导出：立即备份、下载/删除归档、定时备份设置。还原只能在服务器上用命令行做。
import { useI18n } from "@vben/locales";
import { useMediaQuery } from "@vueuse/core";
import {
  NAlert,
  NButton,
  NCard,
  NForm,
  NFormItem,
  NInputNumber,
  NPopconfirm,
  NSwitch,
  NTag,
  useMessage,
} from "naive-ui";
import { computed, onMounted, reactive, shallowRef } from "vue";
import {
  createBackup,
  deleteBackup,
  downloadBackup,
  getBackups,
  saveBackupSchedule,
  type BackupOverview,
} from "@/api/backups";
import { formatDateTime } from "@/components/admin/format";
import LskyImportPanel from "@/components/admin/LskyImportPanel.vue";
import { formatBytes } from "@/lib/format";
import { formatApiError } from "@/locales/errors";

const { t, locale } = useI18n();
const message = useMessage();
const isMobile = useMediaQuery("(max-width: 640px)");

const overview = shallowRef<BackupOverview | null>(null);
const loading = shallowRef(true);
const loadError = shallowRef<unknown>(null);
const creating = shallowRef(false);
const savingSchedule = shallowRef(false);
const pending = reactive(new Set<string>());
const schedule = reactive({ enabled: false, interval_hours: 24, keep: 7 });

const items = computed(() => overview.value?.items ?? []);
const running = computed(() => creating.value || !!overview.value?.running);

function sync(next: BackupOverview): void {
  overview.value = next;
  Object.assign(schedule, next.schedule);
}

async function load(): Promise<void> {
  loading.value = true;
  loadError.value = null;
  try {
    sync(await getBackups());
  } catch (error) {
    loadError.value = error;
  } finally {
    loading.value = false;
  }
}

async function create(): Promise<void> {
  if (running.value) return;
  creating.value = true;
  try {
    const file = await createBackup();
    message.success(t("admin.backups.created", { name: file.name }));
    sync(await getBackups());
  } catch (error) {
    message.error(formatApiError(error, "admin.errors.backupCreate"));
  } finally {
    creating.value = false;
  }
}

async function download(name: string): Promise<void> {
  if (pending.has(name)) return;
  pending.add(name);
  try {
    const blob = await downloadBackup(name);
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = name;
    link.click();
    // 给浏览器一点时间开始下载再释放。
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  } catch (error) {
    message.error(formatApiError(error, "admin.errors.backupDownload"));
  } finally {
    pending.delete(name);
  }
}

async function remove(name: string): Promise<void> {
  if (pending.has(name)) return;
  pending.add(name);
  try {
    await deleteBackup(name);
    message.success(t("admin.backups.deleted"));
    if (overview.value) {
      overview.value = {
        ...overview.value,
        items: overview.value.items.filter((item) => item.name !== name),
      };
    }
  } catch (error) {
    message.error(formatApiError(error, "admin.errors.backupDelete"));
  } finally {
    pending.delete(name);
  }
}

async function saveSchedule(): Promise<void> {
  if (savingSchedule.value || !overview.value) return;
  savingSchedule.value = true;
  try {
    const saved = await saveBackupSchedule({ ...schedule });
    Object.assign(schedule, saved);
    overview.value = { ...overview.value, schedule: saved };
    message.success(t("admin.backups.scheduleSaved"));
  } catch (error) {
    message.error(formatApiError(error, "admin.errors.backupSchedule"));
  } finally {
    savingSchedule.value = false;
  }
}

onMounted(() => void load());
</script>

<template>
  <div class="backup-settings">
    <NCard :bordered="false" :title="t('admin.backups.title')">
      <p class="hint">{{ t("admin.backups.description") }}</p>
      <NAlert type="warning" class="notice">{{ t("admin.backups.secretWarning") }}</NAlert>
      <NAlert v-if="loadError" type="error" class="notice" data-backups-error>
        {{ formatApiError(loadError, "admin.errors.backupsLoad") }}
        <NButton size="tiny" quaternary type="primary" :disabled="loading" @click="load">{{
          t("admin.common.retry")
        }}</NButton>
      </NAlert>
      <div class="toolbar">
        <NButton
          type="primary"
          data-action="create-backup"
          :loading="running"
          :disabled="loading || !overview || running"
          @click="create"
          >{{ running ? t("admin.backups.creating") : t("admin.backups.create") }}</NButton
        >
        <span v-if="overview" class="hint" data-backup-location>{{
          t("admin.backups.location", { dir: overview.location })
        }}</span>
      </div>
      <p v-if="overview && items.length === 0" class="hint" data-backups-empty>
        {{ t("admin.backups.empty") }}
      </p>
      <ul v-else class="backup-list">
        <li v-for="item in items" :key="item.name" class="backup-item" :data-backup="item.name">
          <div class="backup-item__main">
            <span class="backup-item__name">{{ item.name }}</span>
            <span class="hint">
              {{ formatDateTime(item.created_at, locale) }} · {{ formatBytes(item.size) }}
            </span>
          </div>
          <NTag size="small" :type="item.auto ? 'info' : 'default'" :bordered="false">{{
            item.auto ? t("admin.backups.kindAuto") : t("admin.backups.kindManual")
          }}</NTag>
          <div class="backup-item__actions">
            <NButton
              size="small"
              data-action="download-backup"
              :loading="pending.has(item.name)"
              :disabled="pending.has(item.name)"
              @click="download(item.name)"
              >{{ t("admin.backups.download") }}</NButton
            >
            <NPopconfirm
              :positive-text="t('admin.common.delete')"
              :negative-text="t('admin.common.cancel')"
              @positive-click="remove(item.name)"
            >
              <template #trigger>
                <NButton
                  size="small"
                  type="error"
                  quaternary
                  data-action="delete-backup"
                  :disabled="pending.has(item.name)"
                  >{{ t("admin.common.delete") }}</NButton
                >
              </template>
              {{ t("admin.backups.deleteConfirm", { name: item.name }) }}
            </NPopconfirm>
          </div>
        </li>
      </ul>
    </NCard>

    <NCard :bordered="false" :title="t('admin.backups.scheduleTitle')">
      <NForm
        class="schedule-form"
        :label-placement="isMobile ? 'top' : 'left'"
        :label-width="isMobile ? undefined : 150"
        @submit.prevent="saveSchedule"
      >
        <NFormItem :label="t('admin.backups.scheduleEnabled')">
          <NSwitch
            v-model:value="schedule.enabled"
            data-backup-schedule-enabled
            :disabled="!overview"
          />
        </NFormItem>
        <NFormItem :label="t('admin.backups.intervalHours')">
          <NInputNumber
            v-model:value="schedule.interval_hours"
            class="field-control"
            :min="1"
            :max="720"
            :precision="0"
            :disabled="!overview"
            :input-props="{ 'aria-label': t('admin.backups.intervalHours') }"
          />
        </NFormItem>
        <NFormItem :label="t('admin.backups.keep')">
          <div class="field">
            <NInputNumber
              v-model:value="schedule.keep"
              class="field-control"
              :min="1"
              :max="365"
              :precision="0"
              :disabled="!overview"
              :input-props="{ 'aria-label': t('admin.backups.keep') }"
            />
            <p class="hint">{{ t("admin.backups.keepHint") }}</p>
          </div>
        </NFormItem>
        <div class="actions">
          <NButton
            type="primary"
            attr-type="submit"
            data-action="save-backup-schedule"
            :loading="savingSchedule"
            :disabled="!overview || savingSchedule"
            >{{ t("admin.common.save") }}</NButton
          >
        </div>
      </NForm>
    </NCard>

    <NCard :bordered="false" :title="t('admin.backups.restoreTitle')">
      <p class="hint">{{ t("admin.backups.restoreHint") }}</p>
    </NCard>

    <LskyImportPanel />
  </div>
</template>

<style scoped>
.backup-settings {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.hint {
  margin: 0;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}
.notice {
  margin: 12px 0;
}
.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}
.backup-list {
  margin: 0;
  padding: 0;
  list-style: none;
}
.backup-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 0;
  border-top: 1px solid hsl(var(--border));
}
.backup-item__main {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-width: 0;
}
.backup-item__name {
  overflow-wrap: anywhere;
  font-family: ui-monospace, monospace;
}
.backup-item__actions {
  display: flex;
  gap: 4px;
}
.schedule-form,
.actions {
  max-width: 640px;
}
.field,
.field-control {
  width: 100%;
}
.actions {
  display: flex;
  justify-content: flex-end;
}
@media (max-width: 640px) {
  .backup-item {
    flex-wrap: wrap;
  }
}
</style>
