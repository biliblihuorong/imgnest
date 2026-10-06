import { computed, onBeforeUnmount, ref } from "vue";
import { useI18n } from "@vben/locales";
import { ApiError } from "@/api/client";
import {
  MAX_UPLOAD_FILES,
  MAX_UPLOAD_FILE_BYTES,
  MAX_UPLOAD_TOTAL_BYTES,
  preflightUpload,
  uploadImages,
  type UploadItemResult,
} from "@/api/upload";
import { formatBytes } from "@/lib/format";
import type { UploadQueueItem } from "./types";

/** Local queue state; all writes remain in the API transport. */
export function useUploadQueue(
  options: () => { policyId: number | null; albumId: number | null; isPublic: boolean },
) {
  const { t } = useI18n();
  const items = ref<UploadQueueItem[]>([]);
  const uploading = ref(false);
  const rejectedFiles = ref<File[] | null>(null);
  let nextId = 1;
  let active = true;
  let controller: AbortController | undefined;
  function endSession() {
    active = false;
    controller?.abort();
    items.value = [];
    rejectedFiles.value = null;
    uploading.value = false;
  }
  window.addEventListener("imgnest:session-cleared", endSession);
  onBeforeUnmount(() => {
    active = false;
    controller?.abort();
    window.removeEventListener("imgnest:session-cleared", endSession);
  });
  const canUpload = computed(
    () =>
      options().policyId !== null &&
      !uploading.value &&
      items.value.some((item) => item.state === "queued"),
  );
  const preflightErrors = computed(() => {
    const files = rejectedFiles.value;
    if (!files) return [];
    const errors: string[] = [];
    if (!files.length) errors.push(t("user.upload.selectFiles"));
    if (files.length > MAX_UPLOAD_FILES)
      errors.push(t("user.upload.tooMany", { max: MAX_UPLOAD_FILES, count: files.length }));
    const oversized = files.filter((file) => file.size > MAX_UPLOAD_FILE_BYTES);
    if (oversized.length)
      errors.push(
        t("user.upload.fileLimit", {
          limit: formatBytes(MAX_UPLOAD_FILE_BYTES),
          files: oversized.map((file) => `${file.name} (${formatBytes(file.size)})`).join(", "),
        }),
      );
    const total = files.reduce((sum, file) => sum + file.size, 0);
    if (total > MAX_UPLOAD_TOTAL_BYTES)
      errors.push(
        t("user.upload.totalLimit", {
          limit: formatBytes(MAX_UPLOAD_TOTAL_BYTES),
          total: formatBytes(total),
        }),
      );
    return errors;
  });
  function clearPreflight() {
    rejectedFiles.value = null;
  }
  function onFiles(files: File[]) {
    if (uploading.value || !active) return;
    const pending = [
      ...items.value.filter((item) => item.state === "queued").map((item) => item.file),
      ...files,
    ];
    if (!preflightUpload(pending).ok) {
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
  function apply(item: UploadQueueItem | undefined, result: UploadItemResult) {
    if (!active || !item) return;
    if (result.ok) {
      item.state = "success";
      item.image = result.image;
      item.error = null;
    } else {
      item.state = "failed";
      item.error = { code: result.code, message: result.message, status: result.status };
    }
  }
  async function runUpload(targets: UploadQueueItem[]) {
    const settings = options();
    if (uploading.value || settings.policyId === null || !targets.length || !active) return;
    if (!preflightUpload(targets.map((item) => item.file)).ok) {
      rejectedFiles.value = targets.map((item) => item.file);
      return;
    }
    uploading.value = true;
    controller = new AbortController();
    targets.forEach((item) => {
      item.state = "uploading";
      item.error = null;
    });
    try {
      const results = await uploadImages({
        ...settings,
        files: targets.map((item) => item.file),
        signal: controller.signal,
        onFileStart: (index, file) => {
          if (!active) return;
          const item = targets[index];
          if (item) {
            item.state = "uploading";
            item.loaded = 0;
            item.total = file.size;
          }
        },
        onFileProgress: (index, _file, loaded, total) => {
          if (!active) return;
          const item = targets[index];
          if (item) {
            item.loaded = loaded;
            item.total = total;
          }
        },
        onFileSettled: (index, _file, result) => apply(targets[index], result),
      });
      results.forEach((result, index) => apply(targets[index], result));
      // A missing result must never leave a file permanently stuck in flight.
      targets
        .filter((item) => item.state === "uploading")
        .forEach((item) =>
          apply(item, { ok: false, filename: item.name, code: -1, status: 0, message: "" }),
        );
    } catch (error) {
      const failure = error instanceof ApiError ? error : new ApiError(-1, "", 0);
      targets
        .filter((item) => item.state !== "success")
        .forEach((item) =>
          apply(item, {
            ok: false,
            filename: item.name,
            code: failure.code,
            status: failure.status,
            message: failure.message,
          }),
        );
    } finally {
      controller = undefined;
      if (active) uploading.value = false;
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
