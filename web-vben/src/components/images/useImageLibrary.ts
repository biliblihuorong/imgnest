import { useI18n } from "@vben/locales";
import { useMessage, type DropdownOption, type SelectOption } from "naive-ui";
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import { watchDebounced } from "@vueuse/core";
import { ApiError } from "@/api/client";
import { formatApiError } from "@/locales/errors";
import { listAlbums, type AlbumView } from "@/api/albums";
import {
  batchAlbums,
  batchDelete,
  batchPermission,
  deleteImage,
  listImages,
  setImageVisibility,
  type ImageBatchItem,
  type ImageView,
} from "@/api/images";

export type ImageOrder = "newest" | "oldest" | "largest" | "smallest";

export interface ImageLibraryOptions {
  /** 锁定相册（相册详情页）：列表固定按该相册过滤并隐藏相册筛选。 */
  lockedAlbumId?: number;
}

/** 相册筛选下拉里「全部」的哨兵值；对外仍表达为 null（不传 album_id）。 */
export const ALBUM_FILTER_ALL = -1;

export function useImageLibrary(options: ImageLibraryOptions = {}) {
  const { t } = useI18n();
  const message = useMessage();
  const lockedAlbumId = options.lockedAlbumId;
  let requestId = 0;
  let active = true;
  onBeforeUnmount(() => {
    active = false;
    requestId++;
  });

  const sizeOptions = computed(() =>
    [20, 50, 100].map((count) => ({ label: t("user.common.perPage", { count }), value: count })),
  );

  const orderOptions = computed<SelectOption[]>(() => [
    { label: t("user.images.orderNewest"), value: "newest" },
    { label: t("user.images.orderOldest"), value: "oldest" },
    { label: t("user.images.orderLargest"), value: "largest" },
    { label: t("user.images.orderSmallest"), value: "smallest" },
  ]);

  const images = ref<ImageView[]>([]);
  const total = ref(0);
  const page = ref(1);
  const size = ref(20);
  const loading = ref(false);
  const loadError = ref<unknown>(null);
  /** 正在执行写操作（改可见性/删除）的图片 id。 */
  const busyIds = reactive(new Set<number>());
  const drawerShow = ref(false);
  const drawerImage = ref<ImageView | null>(null);

  /** 相册列表（筛选与移动共用）；加载失败时降级为空列表。 */
  const albums = ref<AlbumView[]>([]);
  /** 相册筛选：null=全部（不传参）；0=未归类；正整数=指定相册。 */
  const albumFilter = ref<number | null>(null);

  /* ---------------- 筛选/搜索状态 ---------------- */
  /** 统一搜索：文件名 OR 相机品牌/型号/镜头（后端 q 参数）。 */
  const search = ref("");
  const order = ref<ImageOrder>("newest");
  /** 大小范围按 MB 输入，序列化时换算为字节。 */
  const minSizeMb = ref<number | null>(null);
  const maxSizeMb = ref<number | null>(null);
  /** 上传时间范围（毫秒时间戳，NDatePicker datetimerange）。 */
  const dateRange = ref<[number, number] | null>(null);

  /** 是否有任一筛选生效（相册筛选在锁定模式下视为常量）。 */
  const hasActiveFilters = computed(() => {
    if (lockedAlbumId !== undefined) {
      return (
        search.value.trim() !== "" ||
        order.value !== "newest" ||
        minSizeMb.value !== null ||
        maxSizeMb.value !== null ||
        dateRange.value !== null
      );
    }
    return (
      albumFilter.value !== null ||
      search.value.trim() !== "" ||
      order.value !== "newest" ||
      minSizeMb.value !== null ||
      maxSizeMb.value !== null ||
      dateRange.value !== null
    );
  });

  /** 多选：选中图片 id（不跨页，翻页/筛选时清空）。 */
  const selectedIds = ref<number[]>([]);
  const batchLoading = ref(false);
  /** 批量移动的目标相册；null=未选。 */
  const batchTarget = ref<number | null>(null);

  /** 筛选下拉：显式「全部」+ 未归类 + 各相册。 */
  const albumFilterOptions = computed<SelectOption[]>(() => [
    { label: t("user.images.allAlbums"), value: ALBUM_FILTER_ALL },
    { label: t("user.images.unfiled"), value: 0 },
    ...albums.value.map((album) => ({ label: album.name, value: album.id })),
  ]);

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
    void load();
    void loadAlbums();
  });

  async function load(): Promise<void> {
    const current = ++requestId;
    loading.value = true;
    loadError.value = null;
    try {
      const data = await listImages({
        page: page.value,
        size: size.value,
        album_id: lockedAlbumId ?? (albumFilter.value ?? undefined),
        q: search.value.trim() || undefined,
        order: order.value === "newest" ? undefined : order.value,
        min_size: minSizeMb.value ? minSizeMb.value * 1024 * 1024 : undefined,
        max_size: maxSizeMb.value ? maxSizeMb.value * 1024 * 1024 : undefined,
        from: dateRange.value ? new Date(dateRange.value[0]).toISOString() : undefined,
        to: dateRange.value ? new Date(dateRange.value[1]).toISOString() : undefined,
      });
      if (current !== requestId || !active) return;
      images.value = data.items;
      selectedIds.value = selectedIds.value.filter((id) =>
        data.items.some((item) => item.id === id),
      );
      total.value = data.total;
      page.value = data.page;
    } catch (error) {
      if (current === requestId && active) {
        loadError.value = error;
        images.value = [];
        total.value = 0;
        selectedIds.value = [];
      }
    } finally {
      if (current === requestId) loading.value = false;
    }
  }

  async function loadAlbums(): Promise<void> {
    try {
      const data = await listAlbums({ page: 1, size: 100 });
      if (active) albums.value = data.items;
    } catch (error) {
      if (active) message.error(() => formatApiError(error, "user.images.albumsError"));
    }
  }

  /** 搜索关键词防抖后自动重查。 */
  watchDebounced(
    search,
    () => {
      page.value = 1;
      clearSelection();
      void load();
    },
    { debounce: 350, maxWait: 1000 },
  );

  watch([order, minSizeMb, maxSizeMb, dateRange], () => {
    page.value = 1;
    clearSelection();
    void load();
  });

  function handlePageChange(next: number): void {
    page.value = next;
    clearSelection();
    void load();
  }

  function handleSizeChange(next: number): void {
    size.value = next;
    page.value = 1;
    clearSelection();
    void load();
  }

  /** -1 哨兵翻译回 null（全部=不传 album_id）。 */
  function handleAlbumFilterChange(value: number | null): void {
    albumFilter.value = value === ALBUM_FILTER_ALL ? null : value;
    page.value = 1;
    clearSelection();
    void load();
  }

  /** 清空全部筛选并重查（相册筛选在锁定模式下保持不变）。 */
  function resetFilters(): void {
    search.value = "";
    order.value = "newest";
    minSizeMb.value = null;
    maxSizeMb.value = null;
    dateRange.value = null;
    if (lockedAlbumId === undefined) {
      albumFilter.value = null;
    }
    page.value = 1;
    clearSelection();
    void load();
  }

  /**
   * 切换公开/私有。开关是受控组件：失败时列表项不变，开关自动回滚。
   * 语义提示：私有只从公共列表/画廊隐藏，直链仍可访问。
   */
  async function handleToggle(image: ImageView, isPublic: boolean): Promise<void> {
    if (!active || loading.value || batchLoading.value || busyIds.has(image.id)) return;
    busyIds.add(image.id);
    try {
      const updated = await setImageVisibility(image.id, isPublic);
      if (!active) return;
      images.value = images.value.map((item) => (item.id === updated.id ? updated : item));
      message.success(() =>
        t(isPublic ? "user.images.publicSuccess" : "user.images.privateSuccess"),
      );
    } catch (error) {
      if (active) message.error(() => formatApiError(error, "user.images.visibilityError"));
    } finally {
      busyIds.delete(image.id);
    }
  }

  async function handleRemove(image: ImageView): Promise<void> {
    if (!active || loading.value || batchLoading.value || busyIds.has(image.id)) return;
    busyIds.add(image.id);
    try {
      await deleteImage(image.id);
      if (!active) return;
      message.success(() => t("user.images.deleteSuccess"));
      await load();
    } catch (error) {
      if (active) message.error(() => formatApiError(error, "user.images.deleteError"));
    } finally {
      busyIds.delete(image.id);
    }
  }

  /* ---------------- 多选批量（移动/删除/改公开私有） ---------------- */

  function handleSelect(image: ImageView, checked: boolean): void {
    if (loading.value || batchLoading.value || busyIds.has(image.id)) return;
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
    if (!active || ids.length === 0 || batchLoading.value || busyIds.size > 0 || loading.value) {
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
      await load();
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
    sizeOptions,
    orderOptions,
    images,
    total,
    page,
    size,
    loading,
    loadError,
    busyIds,
    drawerShow,
    drawerImage,
    albumFilter,
    search,
    order,
    minSizeMb,
    maxSizeMb,
    dateRange,
    hasActiveFilters,
    selectedIds,
    batchLoading,
    batchTarget,
    albumFilterOptions,
    batchTargetOptions,
    cardMoveOptions,
    load,
    handlePageChange,
    handleSizeChange,
    handleAlbumFilterChange,
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
