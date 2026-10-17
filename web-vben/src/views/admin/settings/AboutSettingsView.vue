<script setup lang="ts">
// 关于：当前版本、更新检查与项目链接。版本号来自服务端编译进去的 VERSION。
import { useI18n } from "@vben/locales";
import { NAlert, NButton, NCard, NSkeleton, NTag } from "naive-ui";
import { computed, onMounted } from "vue";
import { formatDateTime } from "@/components/admin/format";
import { formatApiError } from "@/locales/errors";
import { useSystemStore } from "@/stores/system";

const { t, locale } = useI18n();
const system = useSystemStore();
const info = computed(() => system.version);

onMounted(() => {
  void system.load();
});

function check(): void {
  void system.load({ refresh: true });
}

const links = computed(() => {
  const data = info.value;
  if (!data) return [];
  return [
    { key: "repository", href: data.repository_url },
    { key: "docs", href: data.docs_url },
    { key: "releases", href: data.releases_url },
    { key: "issues", href: data.issues_url },
  ];
});
</script>

<template>
  <div class="about-settings">
    <NCard :bordered="false" :title="t('admin.settings.about.versionTitle')">
      <NAlert v-if="system.error && !info" type="error">
        {{ formatApiError(system.error, "admin.settings.about.loadError") }}
        <NButton size="tiny" quaternary type="primary" @click="system.load()">{{
          t("admin.common.retry")
        }}</NButton>
      </NAlert>
      <NSkeleton v-else-if="!info" text :repeat="3" />
      <template v-else>
        <div class="about-settings__version">
          <span class="about-settings__current" data-testid="about-current"
            >v{{ info.current }}</span
          >
          <NTag v-if="info.update_available" type="error" :bordered="false" round size="small">
            {{ t("admin.settings.about.updateTag") }}
          </NTag>
          <NTag
            v-else-if="info.latest && !info.check_failed"
            type="success"
            :bordered="false"
            round
            size="small"
          >
            {{ t("admin.settings.about.upToDate") }}
          </NTag>
        </div>

        <NAlert
          v-if="info.update_available && info.latest"
          type="warning"
          class="about-settings__alert"
          data-testid="about-update"
          :title="t('admin.settings.about.updateTitle', { version: info.latest.version })"
        >
          <p class="about-settings__text">
            {{
              t("admin.settings.about.updateBody", {
                date: formatDateTime(info.latest.published_at, locale),
              })
            }}
          </p>
          <a :href="info.latest.url" target="_blank" rel="noopener noreferrer">{{
            t("admin.settings.about.releaseNotes")
          }}</a>
        </NAlert>

        <dl class="about-settings__facts">
          <div>
            <dt>{{ t("admin.settings.about.latest") }}</dt>
            <dd>{{ info.latest ? `v${info.latest.version}` : "-" }}</dd>
          </div>
          <div>
            <dt>{{ t("admin.settings.about.checkedAt") }}</dt>
            <dd>
              <template v-if="!info.check_enabled">{{
                t("admin.settings.about.checkDisabled")
              }}</template>
              <template v-else-if="info.checked_at">{{
                formatDateTime(info.checked_at, locale)
              }}</template>
              <template v-else>-</template>
            </dd>
          </div>
        </dl>
        <p v-if="info.check_failed" class="about-settings__muted">
          {{ t("admin.settings.about.checkFailed") }}
        </p>
        <p v-if="!info.check_enabled" class="about-settings__muted">
          {{ t("admin.settings.about.checkDisabledHint") }}
        </p>
        <NButton
          v-if="info.check_enabled"
          class="about-settings__check"
          :loading="system.loading"
          @click="check"
          >{{ t("admin.settings.about.checkNow") }}</NButton
        >
      </template>
    </NCard>

    <NCard :bordered="false" :title="t('admin.settings.about.linksTitle')">
      <nav class="about-settings__links" :aria-label="t('admin.settings.about.linksTitle')">
        <a
          v-for="link in links"
          :key="link.key"
          :href="link.href"
          target="_blank"
          rel="noopener noreferrer"
          class="about-settings__link"
          :data-link="link.key"
        >
          <span class="about-settings__link-title">{{
            t(`admin.settings.about.links.${link.key}.title`)
          }}</span>
          <span class="about-settings__muted">{{
            t(`admin.settings.about.links.${link.key}.description`)
          }}</span>
        </a>
      </nav>
      <p class="about-settings__muted about-settings__license">
        {{ t("admin.settings.about.license") }}
      </p>
    </NCard>
  </div>
</template>

<style scoped>
.about-settings {
  display: grid;
  gap: 16px;
  min-width: 0;
}

.about-settings__version {
  display: flex;
  gap: 10px;
  align-items: center;
}

.about-settings__current {
  font-size: 28px;
  font-weight: 650;
  font-variant-numeric: tabular-nums;
}

.about-settings__alert {
  margin-top: 16px;
}

.about-settings__text {
  margin: 0 0 4px;
}

.about-settings__facts {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, max-content));
  gap: 8px 40px;
  margin: 16px 0 0;
}

.about-settings__facts dt {
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}

.about-settings__facts dd {
  margin: 2px 0 0;
}

.about-settings__muted {
  margin: 8px 0 0;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}

.about-settings__check {
  margin-top: 16px;
}

.about-settings__links {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.about-settings__link {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 14px 16px;
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
  color: hsl(var(--foreground));
  text-decoration: none;
  transition: border-color 150ms;
}

.about-settings__link:hover,
.about-settings__link:focus-visible {
  border-color: hsl(var(--primary));
}

.about-settings__link .about-settings__muted {
  margin: 0;
}

.about-settings__link-title {
  font-weight: 600;
}

.about-settings__license {
  margin-top: 12px;
}

@media (max-width: 640px) {
  .about-settings__links {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
