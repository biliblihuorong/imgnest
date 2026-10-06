import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch, type Ref } from "vue";
import { getToken } from "@/api/client";
import { suggestAlbums } from "@/api/albums";
import { FIELDS, FORMATS, SORTS, foldText, type Span } from "@/domain/search/query";
export interface SearchSuggestion {
  label: string;
  insert: string;
  span: Span;
}
/** Tolerant cursor scanning is deliberately separate from strict submission parsing. */
export function completionContext(raw: string, caret: number) {
  let start = 0,
    end = raw.length,
    quoted = false;
  for (let i = 0; i < raw.length; i++) {
    if (quoted && raw[i] === "\\") {
      i++;
      continue;
    }
    if (raw[i] === '"') quoted = !quoted;
    if (!quoted && /[\t\n\v\f\r \u00a0\u3000]/.test(raw[i]!)) {
      if (i < caret) start = i + 1;
      else {
        end = i;
        break;
      }
    }
  }
  const fragment = raw.slice(start, caret);
  const colon = fragment.indexOf(":");
  const field = colon < 0 ? "" : foldText(fragment.slice(0, colon));
  if (colon >= 0 && ["format", "extension", "album"].includes(field)) {
    quoted = false;
    let itemStart = start + colon + 1;
    for (let i = itemStart; i < caret; i++) {
      if (quoted && raw[i] === "\\") {
        i++;
        continue;
      }
      if (raw[i] === '"') quoted = !quoted;
      if (!quoted && raw[i] === ",") itemStart = i + 1;
    }
    let itemEnd = end;
    quoted = false;
    for (let i = itemStart; i < end; i++) {
      if (quoted && raw[i] === "\\") {
        i++;
        continue;
      }
      if (raw[i] === '"') quoted = !quoted;
      if (!quoted && raw[i] === ",") {
        itemEnd = i;
        break;
      }
    }
    return {
      field,
      prefix: raw.slice(itemStart, caret).replace(/^"/, "").replace(/"$/, ""),
      span: { start: itemStart, end: itemEnd },
    };
  }
  return {
    field,
    prefix: colon < 0 ? fragment : fragment.slice(colon + 1),
    span: { start: colon < 0 ? start : start + colon + 1, end },
  };
}
export function useSearchSuggestions(options: {
  raw: Ref<string>;
  caret: Ref<number>;
  composing: Ref<boolean>;
  focused: Ref<boolean>;
  scopeAlbumId: Ref<number | string | undefined>;
}) {
  const items = ref<SearchSuggestion[]>([]),
    hasMore = shallowRef(false),
    loading = shallowRef(false),
    failed = shallowRef(false);
  const selected = shallowRef(-1),
    dismissed = shallowRef<string | null>(null);
  const context = computed(() => completionContext(options.raw.value, options.caret.value));
  const visible = computed(
    () =>
      options.focused.value &&
      !options.composing.value &&
      dismissed.value !== options.raw.value &&
      (items.value.length > 0 || loading.value || failed.value),
  );
  let seq = 0,
    timer: ReturnType<typeof setTimeout> | undefined,
    controller: AbortController | undefined,
    page = 1,
    active = true;
  function close() {
    dismissed.value = options.raw.value;
    selected.value = -1;
  }
  function invalidate() {
    ++seq;
    clearTimeout(timer);
    controller?.abort();
    items.value = [];
    hasMore.value = false;
    selected.value = -1;
    loading.value = false;
    failed.value = false;
  }
  async function albums(next = 1) {
    const generation = ++seq,
      token = getToken(),
      ctx = context.value;
    controller?.abort();
    controller = new AbortController();
    const signal = controller.signal;
    loading.value = true;
    failed.value = false;
    try {
      const data = await suggestAlbums(
        ctx.prefix.replace(/^#/, ""),
        next,
        signal,
        options.scopeAlbumId.value === undefined ? undefined : String(options.scopeAlbumId.value),
      );
      if (!active || signal.aborted || generation !== seq || token !== getToken()) return;
      const suggestions = data.items.map((item) => ({
        label: `${item.name} (#${item.id})`,
        insert: `#${item.id}`,
        span: ctx.span,
      }));
      items.value = next === 1 ? suggestions : [...items.value, ...suggestions];
      hasMore.value = data.hasMore;
      page = next;
    } catch {
      if (active && !signal.aborted && generation === seq && token === getToken())
        failed.value = true;
    } finally {
      if (active && generation === seq) loading.value = false;
    }
  }
  watch(
    [options.raw, options.caret, options.composing, options.focused, options.scopeAlbumId],
    () => {
      invalidate();
      if (!options.focused.value || options.composing.value) return;
      const ctx = context.value;
      if (ctx.field === "album") {
        timer = setTimeout(() => {
          void albums();
        }, 250);
        return;
      }
      let choices: readonly string[] = [];
      if (!ctx.field && /^[a-z]+$/i.test(ctx.prefix))
        choices = FIELDS.filter((x) => x.startsWith(foldText(ctx.prefix))).map((x) => `${x}:`);
      else if (["format", "extension"].includes(ctx.field)) choices = FORMATS;
      else if (ctx.field === "sort") choices = SORTS;
      else if (ctx.field === "visibility") choices = ["all", "public", "private"];
      else if (ctx.field === "is") choices = ["public", "private"];
      else if (ctx.field === "order")
        choices = ["earliest", "utmost", "least", "created_at", "created_at-asc"];
      items.value = choices
        .filter((value) => value.startsWith(foldText(ctx.prefix)))
        .map((value) => ({ label: value, insert: value, span: ctx.span }));
    },
    { immediate: true },
  );
  function move(delta: number) {
    dismissed.value = null;
    if (!items.value.length) return;
    selected.value = (selected.value + delta + items.value.length) % items.value.length;
  }
  onMounted(() => window.addEventListener("imgnest:session-cleared", invalidate));
  onBeforeUnmount(() => {
    active = false;
    invalidate();
    window.removeEventListener("imgnest:session-cleared", invalidate);
  });
  return {
    items,
    selected,
    visible,
    hasMore,
    loading,
    failed,
    context,
    close,
    move,
    more: () => albums(page + 1),
    retry: () => albums(),
  };
}
