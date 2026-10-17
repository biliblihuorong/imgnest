<script setup lang="ts">
import { CircleHelp } from "@vben/icons";
import { useI18n } from "@vben/locales";
import { NTooltip } from "naive-ui";
import { computed } from "vue";
import { docUrl, type DocTopicName } from "@/lib/docs";

/**
 * 功能旁边的「?」：新标签页打开对应文档。地址统一在 @/lib/docs 登记，
 * 这里只按当前界面语言取用。
 */
const props = defineProps<{ topic: DocTopicName }>();

const { t, locale } = useI18n();
const href = computed(() => docUrl(props.topic, locale.value));
</script>

<template>
  <NTooltip>
    <template #trigger>
      <a
        :href="href"
        target="_blank"
        rel="noopener noreferrer"
        class="doc-help"
        :aria-label="t('common.docHelp')"
        :data-doc-topic="topic"
        @click.stop
      >
        <CircleHelp class="doc-help__icon" aria-hidden="true" />
      </a>
    </template>
    {{ t("common.docHelp") }}
  </NTooltip>
</template>

<style scoped>
.doc-help {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  margin-inline-start: 4px;
  border-radius: 50%;
  color: hsl(var(--muted-foreground, 0 0% 45%));
  vertical-align: middle;
  transition: color 0.15s;
}
.doc-help:hover,
.doc-help:focus-visible {
  color: hsl(var(--primary, 212 100% 45%));
}
.doc-help__icon {
  width: 16px;
  height: 16px;
}
</style>
