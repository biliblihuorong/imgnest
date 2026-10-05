import { onBeforeUnmount, shallowRef, watch } from "vue";
import { fetchProtectedThumbnail } from "@/api/thumbnails";

/** Own each blob URL and cancel its lifetime when the resource changes or unmounts. */
export function useProtectedThumbnail(getUrl: () => string | null | undefined) {
  const thumbnailUrl = shallowRef("");
  const failed = shallowRef(false);
  let sessionCleared = false;
  let cancelCurrent: () => void = () => {};
  function clearSession() {
    sessionCleared = true;
    cancelCurrent();
    thumbnailUrl.value = "";
    failed.value = false;
  }
  window.addEventListener("imgnest:session-cleared", clearSession);
  onBeforeUnmount(() => window.removeEventListener("imgnest:session-cleared", clearSession));
  watch(
    getUrl,
    async (url, _previous, onCleanup) => {
      const controller = new AbortController();
      let objectUrl = "";
      let current = true;
      thumbnailUrl.value = "";
      failed.value = false;
      cancelCurrent = () => {
        current = false;
        controller.abort();
        if (objectUrl) {
          URL.revokeObjectURL(objectUrl);
          objectUrl = "";
        }
      };
      onCleanup(cancelCurrent);
      if (!url || sessionCleared) return;
      try {
        const blob = await fetchProtectedThumbnail(url, controller.signal);
        if (!current) return;
        objectUrl = URL.createObjectURL(blob);
        thumbnailUrl.value = objectUrl;
      } catch {
        if (current) {
          failed.value = true;
          thumbnailUrl.value = "";
        }
      }
    },
    { immediate: true },
  );
  return { thumbnailUrl, failed };
}
