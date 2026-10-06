import { onMounted, onScopeDispose, reactive, shallowRef } from "vue";
import { useI18n } from "@vben/locales";
import { useMessage } from "naive-ui";
import { deleteAlbum, listAlbums, type AlbumView } from "@/api/albums";
import { formatApiError } from "@/locales/errors";

export const ALBUM_PAGE_SIZE = 20;

/** Own list requests and mutations so only the active page can update the UI. */
export function useAlbums() {
  const { t } = useI18n();
  const message = useMessage();
  const albums = shallowRef<AlbumView[]>([]);
  const total = shallowRef(0);
  const page = shallowRef(1);
  const loading = shallowRef(false);
  const loadError = shallowRef<unknown>(null);
  const busyIds = reactive(new Set<number>());
  let requestId = 0;
  let disposed = false;

  async function load(): Promise<void> {
    const current = ++requestId;
    loading.value = true;
    loadError.value = null;
    albums.value = [];
    try {
      let data = await listAlbums({ page: page.value, size: ALBUM_PAGE_SIZE });
      if (disposed || current !== requestId) return;
      // A deletion in another tab can make this page disappear. Retry only
      // once and retain the same generation so newer navigation always wins.
      const lastPage = Math.max(1, Math.ceil(data.total / ALBUM_PAGE_SIZE));
      if (data.items.length === 0 && data.page > lastPage) {
        data = await listAlbums({ page: lastPage, size: ALBUM_PAGE_SIZE });
      }
      if (disposed || current !== requestId) return;
      albums.value = data.items;
      total.value = data.total;
      page.value = data.page;
    } catch (error) {
      if (!disposed && current === requestId) {
        albums.value = [];
        loadError.value = error;
      }
    } finally {
      if (!disposed && current === requestId) loading.value = false;
    }
  }

  function changePage(next: number): void {
    if (next === page.value && loading.value) return;
    page.value = next;
    void load();
  }

  async function remove(album: AlbumView): Promise<void> {
    if (busyIds.has(album.id) || disposed) return;
    const removedPage = page.value;
    busyIds.add(album.id);
    try {
      await deleteAlbum(album.id);
      if (disposed) return;
      message.success(() => t("albums.deleted"));
      if (
        page.value === removedPage &&
        page.value > 1 &&
        albums.value.length === 1 &&
        albums.value[0]?.id === album.id
      )
        page.value -= 1;
      await load();
    } catch (error) {
      if (!disposed) message.error(() => formatApiError(error, "albums.deleteFailed"));
    } finally {
      busyIds.delete(album.id);
    }
  }

  onMounted(() => void load());
  onScopeDispose(() => {
    disposed = true;
    requestId += 1;
  });
  return { albums, total, page, loading, loadError, busyIds, load, changePage, remove };
}
