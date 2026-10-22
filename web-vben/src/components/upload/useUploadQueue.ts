import { computed, onBeforeUnmount, ref } from "vue";
import { useI18n } from "@vben/locales";
import { ApiError } from "@/api/client";
import {
  MAX_UPLOAD_FILES,
  preflightUpload,
  uploadImages,
  type UploadItemResult,
} from "@/api/upload";
import { formatBytes } from "@/lib/format";
import type { UploadQueueItem } from "./types";

/** Local queue state; all writes remain in the API transport. */
export function useUploadQueue(
  /** guest=true 时走游客上传（公开图床页），不需要选择规则。 */
  options: () => {
    policyId: number | null;
    albumId: number | null;
    isPublic: boolean;
    guest?: boolean;
  },
  /** 当前用户的单文件上限（字节）；null = 还没读到，不在客户端拦。 */
  maxFileBytes: () => number | null = () => null,
) {
  const { t } = useI18n();
  const items = ref<UploadQueueItem[]>([]);
  const uploading = ref(false);
  const rejectedFiles = ref<File[] | null>(null);
  /** 上传进行中拖入或选择的文件不会入队，需要明确告诉用户。 */
  const ignoredWhileUploading = ref(false);
  let nextId = 1;
  let active = true;
  /** 每次会话变化 +1；旧批次的迟到回调按代号丢弃。 */
  let generation = 0;
  let controller: AbortController | undefined;
  function endSession() {
    generation++;
    controller?.abort();
    items.value = [];
    rejectedFiles.value = null;
    ignoredWhileUploading.value = false;
    uploading.value = false;
    // 游客队列不属于任何账号：清空后继续可用（公开首页不随会话重挂载）。
    // 账号队列随布局按会话重建，旧实例从此不再接收文件。
    if (!options().guest) active = false;
  }
  window.addEventListener("imgnest:session-cleared", endSession);
  onBeforeUnmount(() => {
    active = false;
    generation++;
    controller?.abort();
    window.removeEventListener("imgnest:session-cleared", endSession);
  });
  const hasRule = () => options().policyId !== null || options().guest === true;
  const canUpload = computed(
    () => hasRule() && !uploading.value && items.value.some((item) => item.state === "queued"),
  );
  const preflightErrors = computed(() => {
    if (ignoredWhileUploading.value) return [t("user.upload.busyIgnored")];
    const files = rejectedFiles.value;
    if (!files) return [];
    const errors: string[] = [];
    if (!files.length) errors.push(t("user.upload.selectFiles"));
    if (files.length > MAX_UPLOAD_FILES)
      errors.push(t("user.upload.tooMany", { max: MAX_UPLOAD_FILES, count: files.length }));
    const limit = maxFileBytes();
    const oversized = limit === null ? [] : files.filter((file) => file.size > limit);
    if (limit !== null && oversized.length)
      errors.push(
        t("user.upload.fileLimit", {
          limit: formatBytes(limit),
          files: oversized.map((file) => `${file.name} (${formatBytes(file.size)})`).join(", "),
        }),
      );
    return errors;
  });
  function clearPreflight() {
    rejectedFiles.value = null;
    ignoredWhileUploading.value = false;
  }
  function onFiles(files: File[]) {
    if (!active) return;
    if (uploading.value) {
      if (files.length) ignoredWhileUploading.value = true;
      return;
    }
    const pending = [
      ...items.value.filter((item) => item.state === "queued").map((item) => item.file),
      ...files,
    ];
    if (!preflightUpload(pending, maxFileBytes()).ok) {
      rejectedFiles.value = pending;
      return;
    }
    clearPreflight();
    items.value.push(
      ...files.map((file) => ({
        id: nextId++,
        file,
        name: file.name,
        size: file.size,
        state: "queued" as const,
        loaded: 0,
        total: file.size,
        error: null,
        image: null,
      })),
    );
  }
  function apply(item: UploadQueueItem | undefined, result: UploadItemResult, run: number) {
    if (!active || run !== generation || !item) return;
    if (result.ok) {
      item.state = "success";
      item.image = result.image;
      item.error = null;
    } else {
      item.state = "failed";
      item.error = {
        code: result.code,
        message: result.message,
        status: result.status,
        data: result.data,
      };
    }
  }
  async function runUpload(targets: UploadQueueItem[]) {
    const settings = options();
    if (uploading.value || !hasRule() || !targets.length || !active) return;
    if (
      !preflightUpload(
        targets.map((item) => item.file),
        maxFileBytes(),
      ).ok
    ) {
      rejectedFiles.value = targets.map((item) => item.file);
      return;
    }
    uploading.value = true;
    const run = generation;
    const live = () => active && run === generation;
    const runController = new AbortController();
    controller = runController;
    targets.forEach((item) => {
      item.state = "uploading";
      item.error = null;
    });
    try {
      const results = await uploadImages({
        ...settings,
        files: targets.map((item) => item.file),
        signal: runController.signal,
        onFileStart: (index, file) => {
          if (!live()) return;
          const item = targets[index];
          if (item) {
            item.state = "uploading";
            item.loaded = 0;
            item.total = file.size;
          }
        },
        onFileProgress: (index, _file, loaded, total) => {
          if (!live()) return;
          const item = targets[index];
          if (item) {
            item.loaded = loaded;
            item.total = total;
          }
        },
        onFileSettled: (index, _file, result) => apply(targets[index], result, run),
      });
      results.forEach((result, index) => apply(targets[index], result, run));
      // A missing result must never leave a file permanently stuck in flight.
      targets
        .filter((item) => item.state === "uploading")
        .forEach((item) =>
          apply(item, { ok: false, filename: item.name, code: -1, status: 0, message: "" }, run),
        );
    } catch (error) {
      const failure = error instanceof ApiError ? error : new ApiError(-1, "", 0);
      targets
        .filter((item) => item.state !== "success")
        .forEach((item) =>
          apply(
            item,
            {
              ok: false,
              filename: item.name,
              code: failure.code,
              status: failure.status,
              message: failure.message,
            },
            run,
          ),
        );
    } finally {
      if (controller === runController) controller = undefined;
      if (live()) uploading.value = false;
    }
  }
  function startUpload() {
    void runUpload(items.value.filter((item) => item.state === "queued"));
  }
  function retryItem(item: UploadQueueItem) {
    if (item.state === "failed") void runUpload([item]);
  }
  return {
    items,
    uploading,
    canUpload,
    preflightErrors,
    clearPreflight,
    onFiles,
    startUpload,
    retryItem,
  };
}
