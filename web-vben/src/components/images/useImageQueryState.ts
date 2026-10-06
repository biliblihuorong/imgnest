import {
  computed,
  getCurrentInstance,
  inject,
  onBeforeUnmount,
  onMounted,
  ref,
  shallowRef,
  watch,
} from "vue";
import { routerKey, type LocationQuery, type LocationQueryRaw } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import { ApiError, getToken } from "@/api/client";
import {
  searchImages,
  type ImageSearchMetadata,
  type ImageSearchParams,
  type ImageSearchPage,
  type ImageView,
} from "@/api/images";
import { diagnostic, parseQueryV1, type Diagnostic } from "@/domain/search/query";

export function defaultTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  } catch {
    return "UTC";
  }
}
export function validTimezone(value: string): boolean {
  try {
    if (!value || value === "Local" || /^(?:GMT|UTC)[+-]|^[+-]/i.test(value)) return false;
    new Intl.DateTimeFormat("en", { timeZone: value }).format();
    return true;
  } catch {
    return false;
  }
}
export function useImageQueryState(options: {
  lockedAlbumId?: number | string;
  clearSelection?: () => void;
}) {
  const router = inject(routerKey, null);
  const images = ref<ImageView[]>([]),
    total = shallowRef(0),
    page = shallowRef(1),
    size = shallowRef(20);
  const draftRaw = shallowRef(""),
    timezone = shallowRef(defaultTimezone()),
    loading = shallowRef(false);
  const loadError = shallowRef<unknown>(null),
    serverDiagnostics = ref<Diagnostic[]>([]),
    submitted = shallowRef(false);
  const applied = shallowRef<ImageSearchParams | null>(null),
    metadata = shallowRef<ImageSearchMetadata | null>(null);
  const draftParse = computed(() => parseQueryV1(draftRaw.value));
  const diagnostics = computed(() =>
    !draftParse.value.ok ? draftParse.value.diagnostics : serverDiagnostics.value,
  );
  const hasSuccessfulResult = computed(() => applied.value !== null);
  const hasActiveFilters = computed(() => (applied.value?.q ?? "") !== "");
  const unapplied = computed(
    () => draftRaw.value !== (applied.value?.q ?? "") || timezone.value !== applied.value?.tz,
  );
  const staleResults = computed(
    () => hasSuccessfulResult.value && (!!loadError.value || diagnostics.value.length > 0),
  );
  let generation = 0,
    controller: AbortController | undefined,
    active = true;
  let lastAttempt: ImageSearchParams | null = null,
    inFlightKey = "";
  let inFlight: Promise<void> | null = null;
  const internalRoutes = new Set<string>();
  type Mode = "submit" | "route" | "refresh" | "page" | "size" | "retry";
  function params(
    q = applied.value?.q ?? "",
    nextPage = page.value,
    nextSize = size.value,
  ): ImageSearchParams {
    return { qv: 1, q, tz: timezone.value, page: nextPage, size: nextSize };
  }
  async function writeRoute(target: ImageSearchParams, replace: boolean): Promise<void> {
    if (!router) return;
    const query: LocationQueryRaw = {
      ...router.currentRoute.value.query,
      qv: "1",
      q: target.q,
      tz: target.tz,
      page: String(target.page),
      size: String(target.size),
    };
    // Legacy filtering parameters cannot silently coexist with the versioned language.
    for (const key of [
      "keyword",
      "album_id",
      "order",
      "sort",
      "min_size",
      "max_size",
      "from",
      "to",
      "exif",
    ])
      delete query[key];
    const targetRoute = {
      path: router.currentRoute.value.path,
      query,
      hash: router.currentRoute.value.hash,
    };
    const key = router.resolve(targetRoute).fullPath;
    if (key === router.currentRoute.value.fullPath) return;
    internalRoutes.add(key);
    await (replace ? router.replace(targetRoute) : router.push(targetRoute));
    internalRoutes.delete(key);
  }
  function execute(target: ImageSearchParams, mode: Mode): Promise<void> {
    const key = JSON.stringify(target);
    if (inFlight && key === inFlightKey) return inFlight;
    controller?.abort();
    controller = new AbortController();
    const requestController = controller,
      current = ++generation,
      token = getToken(),
      originalDraft = draftRaw.value;
    const currentRequest = () =>
      active && current === generation && !requestController.signal.aborted && getToken() === token;
    lastAttempt = target;
    inFlightKey = key;
    loading.value = true;
    loadError.value = null;
    serverDiagnostics.value = [];
    if (mode !== "refresh") options.clearSelection?.();
    let timedOut = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let abortListener: (() => void) | undefined;
    inFlight = (async () => {
      try {
        const data = await new Promise<ImageSearchPage>((resolve, reject) => {
          abortListener = () =>
            reject(
              timedOut
                ? new ApiError(-1, "Search request timed out", 0)
                : new DOMException("Request cancelled", "AbortError"),
            );
          requestController.signal.addEventListener("abort", abortListener, { once: true });
          timer = setTimeout(() => {
            timedOut = true;
            requestController.abort();
          }, 30000);
          searchImages(target, requestController.signal, options.lockedAlbumId).then(
            resolve,
            reject,
          );
        });
        if (!currentRequest()) return;
        if (
          data.search?.appliedVersion !== 1 ||
          typeof data.search.canonicalQ !== "string" ||
          !validTimezone(data.search.tz)
        ) {
          throw new ApiError(-1, "SEARCH_PROTOCOL_MISMATCH", 502, {
            diagnostics: [diagnostic("SEARCH_PROTOCOL_MISMATCH", 0, originalDraft.length)],
          });
        }
        const canonical = parseQueryV1(data.search.canonicalQ);
        if (!canonical.ok)
          throw new ApiError(-1, "SEARCH_PROTOCOL_MISMATCH", 502, {
            diagnostics: [diagnostic("SEARCH_PROTOCOL_MISMATCH", 0, originalDraft.length)],
          });
        const previous = applied.value;
        const successful = {
          ...target,
          q: data.search.canonicalQ,
          tz: data.search.tz,
          page: data.page,
          size: data.size,
        };
        images.value = data.items;
        total.value = data.total;
        page.value = data.page;
        size.value = data.size;
        applied.value = successful;
        metadata.value = data.search;
        timezone.value = data.search.tz;
        const mayNormalizeDraft =
          mode === "submit" ||
          mode === "route" ||
          (mode === "retry" && draftParse.value.ok && draftRaw.value === target.q);
        if (mayNormalizeDraft && draftRaw.value === originalDraft)
          draftRaw.value = data.search.canonicalQ;
        if (mode !== "route") {
          const changed = !previous || JSON.stringify(previous) !== JSON.stringify(successful);
          await writeRoute(target, mode === "refresh" || !changed);
          if (!currentRequest()) return;
        }
        await writeRoute(successful, true);
      } catch (error) {
        if (
          !currentRequest() &&
          !(timedOut && active && current === generation && getToken() === token)
        )
          return;
        loadError.value = error;
        const data =
          error instanceof ApiError
            ? (error.data as { diagnostics?: Diagnostic[] } | undefined)
            : undefined;
        serverDiagnostics.value =
          draftRaw.value === target.q && Array.isArray(data?.diagnostics)
            ? data.diagnostics.slice(0, 5)
            : [];
      } finally {
        clearTimeout(timer);
        if (abortListener) requestController.signal.removeEventListener("abort", abortListener);
        if (active && current === generation) {
          loading.value = false;
          inFlight = null;
          inFlightKey = "";
        }
      }
    })();
    return inFlight;
  }
  async function submit(): Promise<void> {
    submitted.value = true;
    serverDiagnostics.value = [];
    if (!draftParse.value.ok) {
      controller?.abort();
      ++generation;
      loading.value = false;
      inFlight = null;
      options.clearSelection?.();
      return;
    }
    if (!validTimezone(timezone.value)) {
      serverDiagnostics.value = [diagnostic("INVALID_TIMEZONE", 0, 0)];
      return;
    }
    await execute(params(draftRaw.value, 1), "submit");
  }
  async function resetFilters(): Promise<void> {
    draftRaw.value = "";
    await submit();
  }
  async function load(): Promise<void> {
    if (!applied.value && !lastAttempt) return;
    await execute(applied.value ?? lastAttempt!, "refresh");
  }
  async function retry(): Promise<void> {
    if (lastAttempt) await execute(lastAttempt, "retry");
    else await submit();
  }
  async function handlePageChange(next: number): Promise<void> {
    if (next < 1 || next > 10000) return;
    await execute({ ...(applied.value ?? params()), page: next }, "page");
  }
  async function handleSizeChange(next: number): Promise<void> {
    if (![20, 50, 100].includes(next)) return;
    await execute({ ...(applied.value ?? params()), page: 1, size: next }, "size");
  }
  async function reloadAfterMutation(): Promise<void> {
    await load();
    const lastPage = Math.max(1, Math.ceil(total.value / size.value));
    if (!loadError.value && page.value > lastPage) await handlePageChange(lastPage);
  }
  async function restoreRoute(query: LocationQuery): Promise<void> {
    controller?.abort();
    ++generation;
    inFlight = null;
    loading.value = false;
    submitted.value = true;
    serverDiagnostics.value = [];
    loadError.value = null;
    const one = (key: string, fallback: string) =>
      typeof query[key] === "string"
        ? (query[key] as string)
        : query[key] === undefined
          ? fallback
          : "\0";
    draftRaw.value = one("q", "");
    timezone.value = one("tz", defaultTimezone());
    const p = one("page", "1"),
      s = one("size", "20");
    if (one("qv", "1") !== "1")
      serverDiagnostics.value = [diagnostic("UNSUPPORTED_QUERY_VERSION", 0, 0)];
    else if (!validTimezone(timezone.value))
      serverDiagnostics.value = [diagnostic("INVALID_TIMEZONE", 0, 0)];
    else if (!/^[1-9][0-9]*$/.test(p) || Number(p) > 10000 || !["20", "50", "100"].includes(s))
      serverDiagnostics.value = [diagnostic("INVALID_PAGINATION", 0, 0)];
    else if (
      ["keyword", "album_id", "order", "sort", "min_size", "max_size", "from", "to", "exif"].some(
        (key) => query[key] !== undefined,
      )
    )
      serverDiagnostics.value = [diagnostic("MIXED_QUERY_PROTOCOL", 0, 0)];
    if (serverDiagnostics.value.length || !draftParse.value.ok) {
      options.clearSelection?.();
      return;
    }
    await execute(params(draftRaw.value, Number(p), Number(s)), "route");
  }
  function clearSession(): void {
    controller?.abort();
    ++generation;
    inFlight = null;
    loading.value = false;
    images.value = [];
    total.value = 0;
    applied.value = null;
    metadata.value = null;
    loadError.value = null;
    serverDiagnostics.value = [];
    options.clearSelection?.();
  }
  watch(
    draftRaw,
    () => {
      serverDiagnostics.value = [];
      submitted.value = false;
    },
    { flush: "sync" },
  );
  if (router)
    watch(
      () => router.currentRoute.value.fullPath,
      (path) => {
        if (!internalRoutes.has(path)) void restoreRoute(router.currentRoute.value.query);
      },
    );
  const pinia = getCurrentInstance()?.appContext.config.globalProperties.$pinia;
  if (pinia) {
    const auth = useAuthStore(pinia);
    watch(
      () => [auth.sessionGeneration, auth.token, auth.user?.id],
      () => {
        clearSession();
        if (auth.token) void restoreRoute(router?.currentRoute.value.query ?? {});
      },
    );
  }
  onMounted(() => {
    window.addEventListener("imgnest:session-cleared", clearSession);
    void restoreRoute(router?.currentRoute.value.query ?? {});
  });
  onBeforeUnmount(() => {
    active = false;
    controller?.abort();
    ++generation;
    window.removeEventListener("imgnest:session-cleared", clearSession);
  });
  return {
    images,
    total,
    page,
    size,
    draftRaw,
    draftParse,
    timezone,
    applied,
    metadata,
    loading,
    loadError,
    diagnostics,
    submitted,
    hasSuccessfulResult,
    hasActiveFilters,
    unapplied,
    staleResults,
    submit,
    resetFilters,
    load,
    retry,
    handlePageChange,
    handleSizeChange,
    reloadAfterMutation,
  };
}
