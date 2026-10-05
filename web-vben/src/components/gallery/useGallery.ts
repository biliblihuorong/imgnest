import { onScopeDispose, shallowRef, watch } from "vue";
import { listGallery, type GalleryItem } from "@/api/gallery";
import { useSiteStore } from "@/stores/site";

export const GALLERY_PAGE_SIZE = 30;

/** Public-only gallery state, invalidated immediately when the site switch closes. */
export function useGallery() {
  const site = useSiteStore();
  const items = shallowRef<GalleryItem[]>([]);
  const total = shallowRef(0);
  const page = shallowRef(1);
  const loading = shallowRef(false);
  const loadError = shallowRef(false);
  let requestId = 0;
  let disposed = false;

  async function load(): Promise<void> {
    if (disposed || !site.loaded || !site.galleryEnabled) return;
    const current = ++requestId;
    loading.value = true;
    loadError.value = false;
    items.value = [];
    try {
      const data = await listGallery({ page: page.value, size: GALLERY_PAGE_SIZE });
      if (disposed || current !== requestId || !site.galleryEnabled) return;
      items.value = data.items;
      total.value = data.total;
      page.value = data.page;
    } catch {
      if (!disposed && current === requestId) loadError.value = true;
    } finally {
      if (!disposed && current === requestId) loading.value = false;
    }
  }

  function changePage(next: number): void {
    if (loading.value && next === page.value) return;
    page.value = next;
    void load();
  }

  async function retry(): Promise<void> {
    if (loading.value) return;
    if (!site.loaded) {
      loading.value = true;
      await site.ensureLoaded();
      if (!disposed && !site.loaded) {
        loading.value = false;
        loadError.value = true;
      }
      return;
    }
    await load();
  }

  watch(
    () => [site.loaded, site.galleryEnabled] as const,
    ([loaded, enabled]) => {
      requestId += 1;
      items.value = [];
      total.value = 0;
      page.value = 1;
      loadError.value = false;
      loading.value = !loaded;
      if (loaded && enabled) void load();
    },
    { immediate: true },
  );

  void site.ensureLoaded().then(() => {
    if (!disposed && !site.loaded) {
      loading.value = false;
      loadError.value = true;
    }
  });
  onScopeDispose(() => {
    disposed = true;
    requestId += 1;
  });
  return { site, items, total, page, loading, loadError, changePage, retry };
}
