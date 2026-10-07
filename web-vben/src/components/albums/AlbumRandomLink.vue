<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NAlert, NButton, NInput, NPopconfirm, NSwitch, useMessage } from "naive-ui";
import { computed, onScopeDispose, shallowRef, watch } from "vue";
import {
  deleteRandomLink,
  getRandomLink,
  putRandomLink,
  resetRandomLink,
  type RandomLinkView,
} from "@/api/albums";
import { copyText } from "@/lib/clipboard";
import { formatApiError } from "@/locales/errors";

/**
 * 相册的随机图片链接管理：启用/停用、复制、重置、删除。
 * 链接地址由后端返回的站内路径拼上当前域名得到，不落库。
 */
const props = defineProps<{ albumId: string }>();
const { t } = useI18n();
const message = useMessage();

const link = shallowRef<RandomLinkView | null>(null);
const loading = shallowRef(true);
const loadFailed = shallowRef(false);
const saving = shallowRef(false);
/** 相册切换或卸载后，丢弃仍在途中的旧请求结果。 */
let session = 0;

const url = computed(() => (link.value ? window.location.origin + link.value.path : ""));

async function load(): Promise<void> {
  const current = ++session;
  loading.value = true;
  loadFailed.value = false;
  link.value = null;
  try {
    const result = await getRandomLink(props.albumId);
    if (current !== session) return;
    link.value = result;
  } catch {
    if (current !== session) return;
    loadFailed.value = true;
  } finally {
    if (current === session) loading.value = false;
  }
}

/** 统一的写操作外壳：失败时保留操作前的状态并提示。 */
async function mutate(action: () => Promise<RandomLinkView | null>): Promise<boolean> {
  if (saving.value) return false;
  const current = session;
  saving.value = true;
  try {
    const result = await action();
    if (current !== session) return false;
    link.value = result;
    return true;
  } catch (error) {
    if (current === session) message.error(formatApiError(error, "albums.randomLink.saveFailed"));
    return false;
  } finally {
    if (current === session) saving.value = false;
  }
}

function setEnabled(enabled: boolean): void {
  void mutate(() => putRandomLink(props.albumId, enabled));
}

async function reset(): Promise<void> {
  if (await mutate(() => resetRandomLink(props.albumId))) {
    message.success(t("albums.randomLink.resetDone"));
  }
}

function remove(): void {
  void mutate(() => deleteRandomLink(props.albumId));
}

async function copy(): Promise<void> {
  if (await copyText(url.value)) message.success(t("albums.randomLink.copied"));
  else message.error(t("albums.randomLink.copyFailed"));
}

watch(() => props.albumId, load, { immediate: true });
onScopeDispose(() => {
  session += 1;
});
</script>

<template>
  <section class="album-random-link rounded-lg border border-border bg-card">
    <header class="album-random-link__header">
      <div>
        <h2 class="album-random-link__title">{{ t("albums.randomLink.title") }}</h2>
        <p class="album-random-link__hint text-muted-foreground">
          {{ t("albums.randomLink.description") }}
        </p>
      </div>
      <template v-if="link">
        <span class="text-muted-foreground">
          {{ t(link.enabled ? "albums.randomLink.enabled" : "albums.randomLink.disabled") }}
        </span>
        <NSwitch
          :value="link.enabled"
          :loading="saving"
          :aria-label="t('albums.randomLink.enable')"
          @update:value="setEnabled"
        />
      </template>
    </header>

    <NAlert v-if="loadFailed" type="error" :show-icon="false">
      {{ t("albums.randomLink.loadFailed") }}
      <NButton size="tiny" quaternary @click="load">{{ t("albums.retry") }}</NButton>
    </NAlert>

    <template v-else-if="link">
      <div class="album-random-link__row">
        <NInput :value="url" readonly :aria-label="t('albums.randomLink.title')" />
        <NButton type="primary" secondary @click="copy">{{ t("albums.randomLink.copy") }}</NButton>
      </div>
      <p class="album-random-link__hint text-muted-foreground">
        {{ t("albums.randomLink.formatHint") }} <code>?format=original</code>
      </p>
      <div class="album-random-link__actions">
        <NPopconfirm
          :positive-text="t('albums.randomLink.resetConfirm')"
          :negative-text="t('albums.cancel')"
          @positive-click="reset"
        >
          <template #trigger>
            <NButton size="small" :disabled="saving">{{ t("albums.randomLink.reset") }}</NButton>
          </template>
          {{ t("albums.randomLink.resetWarning") }}
        </NPopconfirm>
        <NPopconfirm
          :positive-text="t('albums.confirmDelete')"
          :negative-text="t('albums.cancel')"
          @positive-click="remove"
        >
          <template #trigger>
            <NButton size="small" type="error" quaternary :disabled="saving">
              {{ t("albums.randomLink.delete") }}
            </NButton>
          </template>
          {{ t("albums.randomLink.deleteWarning") }}
        </NPopconfirm>
      </div>
    </template>

    <NPopconfirm
      v-else-if="!loading"
      :positive-text="t('albums.randomLink.enableConfirm')"
      :negative-text="t('albums.cancel')"
      @positive-click="setEnabled(true)"
    >
      <template #trigger>
        <NButton :loading="saving">{{ t("albums.randomLink.enable") }}</NButton>
      </template>
      <span class="album-random-link__warning">{{ t("albums.randomLink.privacyWarning") }}</span>
    </NPopconfirm>
  </section>
</template>

<style scoped>
.album-random-link {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 16px;
  margin-bottom: 16px;
}
.album-random-link__header {
  display: flex;
  align-items: center;
  gap: 12px;
}
.album-random-link__header > div {
  flex: 1;
  min-width: 0;
}
.album-random-link__title {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
}
.album-random-link__hint {
  margin: 4px 0 0;
  font-size: 13px;
}
.album-random-link__row {
  display: flex;
  gap: 8px;
}
.album-random-link__actions {
  display: flex;
  gap: 8px;
}
.album-random-link__warning {
  display: inline-block;
  max-width: 280px;
}
</style>
