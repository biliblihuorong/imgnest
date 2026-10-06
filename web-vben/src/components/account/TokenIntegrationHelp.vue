<script setup lang="ts">
import { useI18n } from "@vben/locales";
import { NAlert, NCard } from "naive-ui";
const { t } = useI18n();
// Examples never contain a session token or the one-time creation response.
const origin = window.location.origin;
const uploadEndpoint = `${origin}/api/v1/upload`;
</script>
<template>
  <NCard :title="t('account.integration.title')" :bordered="false">
    <p class="token-help__intro">{{ t("account.integration.description") }}</p>
    <dl class="token-help__fields">
      <div>
        <dt>{{ t("account.integration.server") }}</dt>
        <dd>
          <code>{{ origin }}</code>
        </dd>
      </div>
      <div>
        <dt>{{ t("account.integration.upload") }}</dt>
        <dd>
          <code>{{ uploadEndpoint }}</code>
        </dd>
      </div>
      <div>
        <dt>{{ t("account.integration.authorization") }}</dt>
        <dd><code>Bearer YOUR_API_TOKEN</code></dd>
      </div>
      <div>
        <dt>{{ t("account.integration.file") }}</dt>
        <dd><code>file</code></dd>
      </div>
      <div>
        <dt>{{ t("account.integration.response") }}</dt>
        <dd><code>data.links.url</code></dd>
      </div>
    </dl>
    <p class="token-help__hint">{{ t("account.integration.instructions") }}</p>
    <p class="token-help__hint">{{ t("account.integration.privacy") }}</p>
    <NAlert type="info" :show-icon="false">{{ t("account.integration.availability") }}</NAlert>
  </NCard>
</template>
<style scoped>
.token-help__intro {
  margin: 0 0 16px;
}
.token-help__fields {
  display: grid;
  gap: 12px;
  margin: 0;
}
.token-help__fields > div {
  display: grid;
  grid-template-columns: minmax(140px, 180px) minmax(0, 1fr);
  gap: 8px;
}
.token-help__fields dt {
  color: hsl(var(--muted-foreground));
}
.token-help__fields dd {
  margin: 0;
  overflow-wrap: anywhere;
}
.token-help__hint {
  color: hsl(var(--muted-foreground));
  line-height: 1.7;
}
@media (max-width: 640px) {
  .token-help__fields > div {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
