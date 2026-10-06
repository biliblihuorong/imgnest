<script setup lang="ts">
import { computed, nextTick, ref, shallowRef, toRef, useId, useTemplateRef } from "vue";
import { useI18n } from "@vben/locales";
import {
  insertCondition,
  parseQueryV1,
  removeField,
  type Diagnostic,
  type Field,
  type Span,
} from "@/domain/search/query";
import { useSearchSuggestions, type SearchSuggestion } from "./useSearchSuggestions";
import SearchConditionChips from "./SearchConditionChips.vue";
import SearchSyntaxHelp from "./SearchSyntaxHelp.vue";
import { defaultTimezone } from "./useImageQueryState";
const props = defineProps<{
  modelValue: string;
  timezone: string;
  authorizedAlbums: { id: string; name: string }[];
  lockedAlbumId?: number | string;
  lockedAlbumName?: string;
  diagnostics?: Diagnostic[];
  submitted?: boolean;
  unapplied?: boolean;
  staleResults?: boolean;
  loading?: boolean;
}>();
const emit = defineEmits<{ "update:modelValue": [raw: string]; submit: []; clear: [] }>();
const { t } = useI18n();
const id = `image-search-${useId()}`;
const displayTimezone = defaultTimezone();
const input = useTemplateRef<HTMLInputElement>("input");
const composing = shallowRef(false),
  focused = shallowRef(false),
  caret = shallowRef(props.modelValue.length);
const selection = ref<Span>({ start: props.modelValue.length, end: props.modelValue.length });
const parsed = computed(() =>
  composing.value ? { ok: false as const, diagnostics: [] } : parseQueryV1(props.modelValue),
);
const firstDiagnostic = computed(() =>
  composing.value
    ? undefined
    : (props.diagnostics?.[0] ?? (!parsed.value.ok ? parsed.value.diagnostics[0] : undefined)),
);
const diagnosticText = computed(() =>
  firstDiagnostic.value ? t(firstDiagnostic.value.messageKey, firstDiagnostic.value.args) : "",
);
const suggestions = useSearchSuggestions({
  raw: toRef(props, "modelValue"),
  caret,
  composing,
  focused,
  scopeAlbumId: toRef(props, "lockedAlbumId"),
});
const hintKey = computed(() => {
  const field = suggestions.context.value.field;
  return field === "camera"
    ? "search.cameraHint"
    : ["minsize", "maxsize"].includes(field)
      ? "search.sizeHint"
      : ["after", "before"].includes(field)
        ? "search.dateHint"
        : "";
});
function captureSelection() {
  caret.value = input.value?.selectionStart ?? props.modelValue.length;
  selection.value = { start: caret.value, end: input.value?.selectionEnd ?? caret.value };
}
function update(event: Event) {
  emit("update:modelValue", (event.target as HTMLInputElement).value);
  if (!composing.value) captureSelection();
}
async function focusRange(span?: Span) {
  await nextTick();
  input.value?.focus();
  if (span) input.value?.setSelectionRange(span.start, span.end);
  captureSelection();
}
async function setDraft(raw: string, position: number) {
  emit("update:modelValue", raw);
  await focusRange({ start: position, end: position });
}
async function choose(suggestion: SearchSuggestion) {
  const raw =
    props.modelValue.slice(0, suggestion.span.start) +
    suggestion.insert +
    props.modelValue.slice(suggestion.span.end);
  await setDraft(raw, suggestion.span.start + suggestion.insert.length);
  suggestions.close();
}
function onKey(event: KeyboardEvent) {
  if (composing.value || event.isComposing || event.keyCode === 229) return;
  if (event.key === "Escape" && suggestions.visible.value) {
    event.preventDefault();
    event.stopPropagation();
    suggestions.close();
    return;
  }
  if (["ArrowDown", "ArrowUp"].includes(event.key) && suggestions.items.value.length > 0) {
    event.preventDefault();
    suggestions.move(event.key === "ArrowDown" ? 1 : -1);
    return;
  }
  if (event.key === "Enter") {
    event.preventDefault();
    const selected = suggestions.visible.value
      ? suggestions.items.value[suggestions.selected.value]
      : undefined;
    if (selected) void choose(selected);
    else {
      suggestions.close();
      emit("submit");
    }
  }
}
function compositionEnd(event: CompositionEvent) {
  composing.value = false;
  update(event);
  captureSelection();
}
function remove(field: Field) {
  const result = removeField(props.modelValue, field);
  void setDraft(result.raw, result.caret);
}
function edit(field: Field) {
  if (parsed.value.ok)
    void focusRange(parsed.value.tokens.find((x) => x.field === field)?.valueSpan);
}
function insert(query: string) {
  const result = insertCondition(props.modelValue, query, selection.value);
  void setDraft(result.raw, result.caret);
}
function replace(query: string) {
  void setDraft(query, query.length);
}
function locate() {
  if (firstDiagnostic.value) void focusRange(firstDiagnostic.value.span);
}
</script>
<template>
  <section class="unified-search" :aria-label="t('search.title')">
    <p
      v-if="lockedAlbumId !== undefined"
      class="unified-search__boundary"
      data-testid="fixed-album-scope"
    >
      {{
        t("search.fixedAlbum", { name: lockedAlbumName || `#${lockedAlbumId}`, id: lockedAlbumId })
      }}
    </p>
    <form class="unified-search__form" @submit.prevent="!composing && emit('submit')">
      <div class="unified-search__input-wrap">
        <input
          :id="id"
          ref="input"
          class="unified-search__input"
          type="text"
          :value="modelValue"
          :placeholder="t('search.placeholder')"
          :aria-label="t('search.title')"
          :aria-describedby="`${id}-status ${id}-error`"
          :aria-invalid="!!firstDiagnostic && submitted"
          role="combobox"
          aria-autocomplete="list"
          :aria-expanded="suggestions.visible.value"
          :aria-controls="`${id}-suggestions`"
          :aria-activedescendant="
            suggestions.visible.value && suggestions.selected.value >= 0
              ? `${id}-option-${suggestions.selected.value}`
              : undefined
          "
          autocomplete="off"
          spellcheck="false"
          @input="update"
          @click="captureSelection"
          @select="captureSelection"
          @keyup="captureSelection"
          @keydown="onKey"
          @focus="
            focused = true;
            captureSelection();
          "
          @blur="focused = false"
          @compositionstart="
            composing = true;
            suggestions.close();
          "
          @compositionend="compositionEnd"
        />
      </div>
      <div class="unified-search__actions">
        <button
          type="button"
          class="unified-search__button"
          :aria-label="t('search.clear')"
          @click="emit('clear')"
        >
          {{ t("search.clear") }}
        </button>
        <SearchSyntaxHelp
          :id="`${id}-help`"
          :raw="modelValue"
          :timezone="timezone"
          @insert="insert"
          @replace="replace"
          @focus-input="focusRange(selection)"
        />
        <button
          type="submit"
          class="unified-search__button unified-search__submit"
          :aria-busy="loading"
        >
          {{ loading ? t("search.searching") : t("search.submit") }}
        </button>
      </div>
    </form>
    <div v-if="suggestions.visible.value" class="unified-search__suggestions">
      <ul :id="`${id}-suggestions`" role="listbox" :aria-label="t('search.suggestions')">
        <li
          v-for="(suggestion, index) in suggestions.items.value"
          :id="`${id}-option-${index}`"
          :key="`${suggestion.insert}-${index}`"
          role="option"
          :aria-selected="index === suggestions.selected.value"
          @mousedown.prevent
          @click="choose(suggestion)"
        >
          {{ suggestion.label }}
        </li>
      </ul>
      <p v-if="suggestions.loading.value" role="status">{{ t("search.suggestionsLoading") }}</p>
      <button
        v-if="suggestions.failed.value"
        type="button"
        @mousedown.prevent
        @click="suggestions.retry"
      >
        {{ t("search.suggestionsRetry") }}
      </button>
      <button
        v-if="suggestions.hasMore.value"
        type="button"
        @mousedown.prevent
        @click="suggestions.more"
      >
        {{ t("search.moreAlbums") }}
      </button>
    </div>
    <p v-if="hintKey && focused" class="unified-search__hint">{{ t(hintKey) }}</p>
    <SearchConditionChips
      :parsed="parsed"
      :authorized-albums="authorizedAlbums"
      @edit="edit"
      @remove="remove"
    />
    <p :id="`${id}-status`" class="unified-search__status" aria-live="polite">
      <span v-if="staleResults">{{ t("search.previousResults") }}</span>
      <span v-else-if="unapplied">{{ t("search.unapplied") }}</span>
      <span>{{ t("search.timezone", { timezone }) }}</span>
      <span v-if="timezone !== displayTimezone">{{
        t("search.displayTimezoneExact", { timezone: displayTimezone })
      }}</span>
    </p>
    <p
      :id="`${id}-error`"
      class="unified-search__error"
      :class="{ 'unified-search__error--warning': !submitted }"
      aria-live="polite"
    >
      <template v-if="firstDiagnostic"
        >{{ diagnosticText }}
        <span>{{
          t("search.position", {
            start: firstDiagnostic.span.start + 1,
            end: Math.max(firstDiagnostic.span.start + 1, firstDiagnostic.span.end),
          })
        }}</span>
        <button type="button" @click="locate">{{ t("search.locate") }}</button></template
      >
    </p>
  </section>
</template>
<style scoped>
.unified-search {
  min-width: 0;
  margin-bottom: 12px;
}
.unified-search__form {
  display: flex;
  gap: 8px;
  align-items: stretch;
  min-width: 0;
}
.unified-search__input-wrap {
  flex: 1;
  min-width: 0;
}
.unified-search__input {
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  height: 44px;
  padding: 10px 12px;
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
  background: hsl(var(--background));
  color: inherit;
  font: inherit;
}
.unified-search__input:focus {
  border-color: hsl(var(--primary));
  outline: 2px solid hsl(var(--primary) / 0.25);
}
.unified-search__actions {
  display: flex;
  gap: 6px;
}
.unified-search__button {
  min-width: 44px;
  min-height: 44px;
  padding: 8px 14px;
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
}
.unified-search__submit {
  background: hsl(var(--primary));
  color: hsl(var(--primary-foreground));
}
.unified-search__button:focus-visible,
.unified-search__error button:focus-visible {
  outline: 2px solid hsl(var(--primary));
  outline-offset: 2px;
}
.unified-search__boundary {
  margin-bottom: 8px;
  padding: 8px 12px;
  border-inline-start: 3px solid hsl(var(--primary));
  background: hsl(var(--muted) / 0.5);
  overflow-wrap: anywhere;
}
.unified-search__status {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  margin-top: 8px;
  color: hsl(var(--muted-foreground));
  font-size: 12px;
}
.unified-search__error {
  color: hsl(var(--destructive));
  overflow-wrap: anywhere;
  font-size: 13px;
}
.unified-search__error--warning {
  color: hsl(var(--muted-foreground));
}
.unified-search__error button {
  text-decoration: underline;
  min-height: 32px;
  padding: 4px;
}
.unified-search__hint {
  margin-top: 6px;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}
.unified-search__suggestions {
  border: 1px solid hsl(var(--border));
  background: hsl(var(--background));
  border-radius: 8px;
  max-height: min(35dvh, 280px);
  overflow: auto;
  margin-top: 4px;
}
.unified-search__suggestions ul {
  margin: 0;
  padding: 4px;
  list-style: none;
}
.unified-search__suggestions li {
  min-height: 44px;
  padding: 10px 12px;
  cursor: pointer;
  overflow-wrap: anywhere;
}
.unified-search__suggestions li[aria-selected="true"],
.unified-search__suggestions li:hover {
  background: hsl(var(--muted));
}
.unified-search__suggestions button {
  min-height: 44px;
  padding: 8px 12px;
  text-decoration: underline;
}
@media (max-width: 600px) {
  .unified-search__form {
    flex-direction: column;
  }
  .unified-search__actions {
    justify-content: flex-end;
  }
  .unified-search__error button {
    min-height: 44px;
  }
}
</style>
