import { onScopeDispose, shallowReadonly, shallowRef, watch } from "vue";
import { listAlbums, type AlbumView } from "@/api/albums";
import { useAuthStore } from "@/stores/auth";

export const SIDEBAR_ALBUM_LIMIT = 20;

/** 侧栏相册列表：跟随当前账号，账号变化即清空重载；过期响应一律丢弃。 */
export function useSidebarAlbums() {
  const auth = useAuthStore();
  const albums = shallowRef<AlbumView[]>([]);
  const total = shallowRef(0);
  const loading = shallowRef(false);
  const failed = shallowRef(false);
  let requestId = 0;
  let disposed = false;

  function signedIn(): boolean {
    return Boolean(auth.token && auth.user);
  }

  async function load(): Promise<void> {
    const current = ++requestId;
    if (!signedIn()) {
      loading.value = false;
      return;
    }
    loading.value = true;
    failed.value = false;
    try {
      const data = await listAlbums({ page: 1, size: SIDEBAR_ALBUM_LIMIT });
      if (disposed || current !== requestId) return;
      albums.value = data.items;
      total.value = data.total;
    } catch {
      if (disposed || current !== requestId) return;
      failed.value = true;
    } finally {
      if (!disposed && current === requestId) loading.value = false;
    }
  }

  watch(
    () => (auth.token ? auth.user?.id : undefined),
    () => {
      albums.value = [];
      total.value = 0;
      failed.value = false;
      void load();
    },
    { immediate: true },
  );

  onScopeDispose(() => {
    disposed = true;
    requestId += 1;
  });

  return {
    albums: shallowReadonly(albums),
    total: shallowReadonly(total),
    loading: shallowReadonly(loading),
    failed: shallowReadonly(failed),
    load,
  };
}
