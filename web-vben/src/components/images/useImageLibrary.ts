import { useI18n } from "@vben/locales";
import { useMessage, type DropdownOption, type SelectOption } from "naive-ui";
import { computed, onBeforeUnmount, onMounted, reactive, ref } from "vue";
import { useImageQueryState } from "./useImageQueryState";
import { ApiError, getToken } from "@/api/client";
import { formatApiError } from "@/locales/errors";
import { listAlbums, type AlbumView } from "@/api/albums";
import {
  batchAlbums,
  batchDelete,
  batchPermission,
  deleteImage,
  setImageVisibility,
  type ImageBatchItem,
  type ImageView,
} from "@/api/images";

export interface ImageLibraryOptions {
  /** Fixed, server-enforced route boundary; never represented by a removable query chip. */
  lockedAlbumId?: number | string;
}

export function useImageLibrary(options: ImageLibraryOptions = {}) {
  const { t } = useI18n();
  const message = useMessage();
  const lockedAlbumId = options.lockedAlbumId;
  let active = true;
  onBeforeUnmount(() => {
    active = false;
  });

  const sizeOptions = computed(() =>
    [20, 50, 100].map((count) => ({ label: t("user.common.perPage", { count }), value: count })),
  );

  const query = useImageQueryState({ lockedAlbumId, clearSelection });
  const {
    images,
    total,
    page,
    size,
    loading,
    loadError,
    load,
    handlePageChange,
    handleSizeChange,
    resetFilters,
    hasActiveFilters,
  } = query;
  /** 正在执行写操作（改可见性/删除）的图片 id。 */
  const busyIds = reactive(new Set<number>());
  const drawerShow = ref(false);
  const drawerImage = ref<ImageView | null>(null);

  /** 相册列表（筛选与移动共用）；加载失败时降级为空列表。 */
  const albums = ref<AlbumView[]>([]);
  /** 多选：选中图片 id（不跨页，翻页/筛选时清空）。 */
  const selectedIds = ref<number[]>([]);
  const batchLoading = ref(false);
  /** 批量移动的目标相册；null=未选。 */
  const batchTarget = ref<number | null>(null);

  /** 批量移动的目标选项：0=移出相册 + 各相册。 */
  const batchTargetOptions = computed<SelectOption[]>(() => [
    { label: t("user.images.removeAlbum"), value: 0 },
    ...albums.value.map((album) => ({ label: album.name, value: album.id })),
  ]);

  /** 卡片「移动」下拉选项（key=相册 id，0=移出相册）。 */
  const cardMoveOptions = computed<DropdownOption[]>(() => [
    { label: t("user.images.removeAlbum"), key: 0 },
    ...albums.value.map((album) => ({ label: album.name, key: album.id })),
  ]);

  onMounted(() => {
    void loadAlbums();
  });

  async function loadAlbums(): Promise<void> {
    const token = getToken();
    try {
      const data = await listAlbums({ page: 1, size: 100 });
      if (active && getToken() === token) albums.value = data.items;
    } catch (error) {
      if (active) message.error(() => formatApiError(error, "user.images.albumsError"));
    }
  }

  /**
   * 切换公开/私有。开关是受控组件：失败时列表项不变，开关自动回滚。
   * 语义提示：私有只从公共列表/画廊隐藏，直链仍可访问。
   */
  async function handleToggle(image: ImageView, isPublic: boolean): Promise<void> {
    if (
      !active ||
      loading.value ||
      loadError.value !== null ||
      query.staleResults.value ||
      batchLoading.value ||
      busyIds.has(image.id)
    )
      return;
    busyIds.add(image.id);
    try {
      const updated = await setImageVisibility(image.id, isPublic);
      if (!active) return;
      images.value = images.value.map((item) => (item.id === updated.id ? updated : item));
      message.success(() =>
        t(isPublic ? "user.images.publicSuccess" : "user.images.privateSuccess"),
      );
      // Visibility participates in server-side filtering; refresh COUNT and clamp after removal.
      await query.reloadAfterMutation();
    } catch (error) {
      if (active) message.error(() => formatApiError(error, "user.images.visibilityError"));
    } finally {
      busyIds.delete(image.id);
    }
  }

  async function handleRemove(image: ImageView): Promise<void> {
    if (
      !active ||
      loading.value ||
      loadError.value !== null ||
      query.staleResults.value ||
      batchLoading.value ||
      busyIds.has(image.id)
    )
      return;
    busyIds.add(image.id);
    try {
      await deleteImage(image.id);
      if (!active) return;
      message.success(() => t("user.images.deleteSuccess"));
      await query.reloadAfterMutation();
    } catch (error) {
      if (active) message.error(() => formatApiError(error, "user.images.deleteError"));
    } finally {
      busyIds.delete(image.id);
    }
  }

  /* ---------------- 多选批量（移动/删除/改公开私有） ---------------- */

  function handleSelect(image: ImageView, checked: boolean): void {
    if (
      loading.value ||
      loadError.value !== null ||
      query.staleResults.value ||
      batchLoading.value ||
      busyIds.has(image.id)
    )
      return;
    selectedIds.value = checked
      ? [...new Set([...selectedIds.value, image.id])]
      : selectedIds.value.filter((id) => id !== image.id);
  }

  function clearSelection(): void {
    selectedIds.value = [];
  }

  /**
   * 执行批量操作：207 逐项结果逐项反馈；成功后刷新列表。
   * 默认清空多选（卡片单图移动等保留选择的场景传 clearAfter: false）。
   */
  async function runBatch(
    action: (ids: number[]) => Promise<ImageBatchItem[]>,
    ids: number[],
    label: string,
    options: { clearAfter?: boolean } = {},
  ): Promise<void> {
    if (
      !active ||
      ids.length === 0 ||
      batchLoading.value ||
      busyIds.size > 0 ||
      loading.value ||
      loadError.value !== null ||
      query.staleResults.value
    ) {
      return;
    }
    batchLoading.value = true;
    try {
      const results = await action(ids);
      if (!active) return;
      const failures = results.filter((item) => item.code !== 0);
      const successCount = results.length - failures.length;
      if (successCount > 0) {
        message.success(() =>
          t("user.common.batchSuccess", { action: t(label), count: successCount }),
        );
      }
      for (const item of failures) {
        message.error(() =>
          t("user.common.batchFailure", {
            action: t(label),
            id: item.id ?? t("user.common.unknown"),
            error: formatApiError(
              new ApiError(item.code, item.message, item.status),
              "user.common.loadError",
            ),
          }),
        );
      }
      await query.reloadAfterMutation();
      if (options.clearAfter !== false) {
        clearSelection();
      }
    } catch (error) {
      if (active)
        message.error(() =>
          t("user.common.actionFailure", {
            action: t(label),
            error: formatApiError(error, "user.common.loadError"),
          }),
        );
    } finally {
      batchLoading.value = false;
    }
  }

  function moveSelected(): void {
    const target = batchTarget.value;
    if (target === null) {
      message.warning(() => t("user.images.selectTarget"));
      return;
    }
    void runBatch((ids) => batchAlbums(ids, target), [...selectedIds.value], "user.common.move");
  }

  function removeSelected(): void {
    void runBatch(batchDelete, [...selectedIds.value], "user.common.remove");
  }

  function batchVisibility(isPublic: boolean): void {
    void runBatch(
      (ids) => batchPermission(ids, isPublic),
      [...selectedIds.value],
      isPublic ? "user.images.makePublic" : "user.images.makePrivate",
    );
  }

  function handleMove(image: ImageView, albumId: number): void {
    void runBatch((ids) => batchAlbums(ids, albumId), [image.id], "user.common.move", {
      clearAfter: false,
    });
  }

  function openDrawer(image: ImageView): void {
    drawerImage.value = image;
    drawerShow.value = true;
  }

  return {
    ...query,
    sizeOptions,
    images,
    total,
    page,
    size,
    loading,
    loadError,
    busyIds,
    drawerShow,
    drawerImage,
    hasActiveFilters,
    selectedIds,
    batchLoading,
    batchTarget,
    batchTargetOptions,
    cardMoveOptions,
    load,
    handlePageChange,
    handleSizeChange,
    resetFilters,
    handleToggle,
    handleRemove,
    handleSelect,
    clearSelection,
    moveSelected,
    removeSelected,
    batchVisibility,
    handleMove,
    openDrawer,
  };
}
