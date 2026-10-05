<script setup lang="ts">
import { computed, nextTick, shallowRef, useTemplateRef } from "vue";
import { NModal, NTooltip } from "naive-ui";
import { useI18n } from "@vben/locales";
import { FIELDS } from "@/domain/search/query";
const props = defineProps<{ raw: string; timezone: string; id: string }>();
const emit = defineEmits<{ insert: [query: string]; replace: [query: string]; focusInput: [] }>();
const { t } = useI18n();
let returnToInput = false;
const opened = shallowRef(false),
  tooltip = shallowRef(false),
  pending = shallowRef<string | null>(null);
const trigger = useTemplateRef<HTMLButtonElement>("trigger");
const examples = [
  "format:jpg,png sort:newest",
  'camera:"Canon EOS" minsize:1MB maxsize:20MB',
  "after:2026-10-01 before:2026-11-01 visibility:private",
  'album:"旅行" sort:oldest',
];
const conditions = computed(() => [
  { syntax: "format:jpg", field: "format" },
  { syntax: 'album:"旅行"', field: "album" },
  { syntax: "after:2026-10-01", field: "after" },
  { syntax: "sort:oldest", field: "sort" },
]);
function open() {
  tooltip.value = false;
  pending.value = null;
  opened.value = true;
}
async function closed() {
  pending.value = null;
  await nextTick();
  if (returnToInput) {
    returnToInput = false;
    emit("focusInput");
  } else trigger.value?.focus();
}
function insert(query: string) {
  returnToInput = true;
  emit("insert", query);
  opened.value = false;
}
function useExample(query: string) {
  if (props.raw.trim()) pending.value = query;
  else replace(query);
}
function replace(query: string) {
  returnToInput = true;
  emit("replace", query);
  opened.value = false;
  pending.value = null;
}
</script>
<template>
  <NTooltip trigger="manual" :show="tooltip && !opened">
    <template #trigger>
      <button
        :id="`${id}-trigger`"
        ref="trigger"
        type="button"
        class="search-help-button"
        data-testid="search-help"
        :aria-label="t('search.helpTitle')"
        :aria-expanded="opened"
        :aria-controls="id"
        aria-haspopup="dialog"
        @mouseenter="tooltip = true"
        @mouseleave="tooltip = false"
        @focus="tooltip = true"
        @blur="tooltip = false"
        @click="open"
      >
        ?
      </button>
    </template>
    {{ t("search.helpTooltip") }}
  </NTooltip>
  <NModal
    v-model:show="opened"
    preset="card"
    :title="t('search.helpTitle')"
    class="search-help-panel"
    :aria-labelledby="`${id}-heading`"
    :auto-focus="true"
    :trap-focus="true"
    :close-on-esc="true"
    @after-leave="closed"
  >
    <div :id="id" class="search-help-content">
      <h2 :id="`${id}-heading`" class="search-help-heading">{{ t("search.helpIntro") }}</h2>
      <p>{{ t("search.timezone", { timezone }) }}</p>
      <p class="search-help-note">{{ t("search.displayTimezone") }}</p>
      <div class="search-help-conditions">
        <div v-for="condition in conditions" :key="condition.field" class="search-help-condition">
          <strong>{{ t(`search.fields.${condition.field}`) }}</strong>
          <code>{{ condition.syntax }}</code>
          <button type="button" @click="insert(condition.syntax)">
            {{ t("search.insertCondition") }}
          </button>
        </div>
      </div>
      <h3>{{ t("search.examples") }}</h3>
      <div v-for="(example, index) in examples" :key="example" class="search-help-example">
        <code>{{ example }}</code>
        <button type="button" :data-example="index" @click="useExample(example)">
          {{ t("search.useExample") }}
        </button>
      </div>
      <p>{{ t("search.albumTemplate") }}</p>
      <div v-if="pending" class="search-help-confirm" role="alert">
        <p>{{ t("search.replaceWarning") }}</p>
        <button type="button" data-confirm-example @click="replace(pending)">
          {{ t("search.confirmReplace") }}
        </button>
        <button type="button" @click="pending = null">{{ t("search.cancel") }}</button>
      </div>
      <details>
        <summary>{{ t("search.allFields") }}</summary>
        <dl class="search-help-fields">
          <template v-for="field in FIELDS" :key="field"
            ><dt>{{ field }}</dt>
            <dd>{{ t(`search.explain.${field}`) }}</dd></template
          >
        </dl>
        <p>{{ t("search.quoting") }}</p>
        <p>{{ t("search.limits") }}</p>
        <p>{{ t("search.dateBoundary") }}</p>
        <p>{{ t("search.security") }}</p>
        <h3>{{ t("search.aliasTitle") }}</h3>
        <p>{{ t("search.aliasIntro") }}</p>
        <ul class="search-help-aliases">
          <li>extension:jpg → format:jpg</li>
          <li>is:private → visibility:private</li>
          <li>order:earliest → sort:oldest</li>
          <li>order:utmost → sort:size-desc</li>
          <li>order:least → sort:size-asc</li>
          <li>order:created_at → sort:newest</li>
          <li>order:created_at-asc → sort:oldest</li>
        </ul>
        <p>{{ t("search.aliasRules") }}</p>
      </details>
      <button type="button" class="search-help-close" @click="opened = false">
        {{ t("search.closeHelp") }}
      </button>
    </div>
  </NModal>
</template>
<style scoped>
.search-help-button {
  width: 44px;
  min-width: 44px;
  height: 44px;
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
  font-weight: 700;
}
.search-help-button:focus-visible,
.search-help-content button:focus-visible {
  outline: 2px solid hsl(var(--primary));
  outline-offset: 2px;
}
.search-help-panel {
  width: min(650px, calc(100vw - 24px));
  max-height: calc(100dvh - 32px);
  margin: 16px auto;
  overflow: auto;
}
.search-help-content {
  display: grid;
  gap: 12px;
  min-width: 0;
}
.search-help-heading {
  font-size: 15px;
}
.search-help-note {
  color: hsl(var(--muted-foreground));
  font-size: 12px;
}
.search-help-condition,
.search-help-example {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
  padding: 8px 0;
}
.search-help-content code {
  overflow-wrap: anywhere;
  white-space: pre-wrap;
  flex: 1;
  min-width: 120px;
}
.search-help-content button {
  min-height: 44px;
  padding: 6px 10px;
  border: 1px solid hsl(var(--border));
  border-radius: 6px;
}
.search-help-confirm {
  background: hsl(var(--muted));
  padding: 12px;
  border-radius: 8px;
}
.search-help-fields {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 8px;
  margin: 12px 0;
}
.search-help-fields dd {
  min-width: 0;
  overflow-wrap: anywhere;
}
.search-help-content summary {
  cursor: pointer;
  padding: 12px 0;
}
.search-help-content details p {
  margin: 10px 0;
}
.search-help-aliases {
  padding-inline-start: 20px;
}
.search-help-close {
  position: sticky;
  bottom: 0;
  background: hsl(var(--background));
}
@media (max-width: 600px) {
  .search-help-panel {
    width: 100vw;
    margin: 0;
    max-height: 85dvh;
    position: fixed;
    bottom: 0;
    border-radius: 16px 16px 0 0;
  }
  .search-help-fields {
    grid-template-columns: 1fr;
  }
}
</style>
