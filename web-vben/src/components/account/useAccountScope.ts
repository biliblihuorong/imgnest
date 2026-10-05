import { onActivated, onBeforeUnmount, onDeactivated, shallowRef } from "vue";

/** Discard late results and clear account secrets when their page/session ends. */
export function useAccountScope(clear: () => void) {
  const available = shallowRef(true);
  let version = 0;
  let sessionEnded = false;
  function invalidate() {
    version += 1;
    available.value = false;
    clear();
  }
  function endSession() {
    sessionEnded = true;
    invalidate();
  }
  window.addEventListener("imgnest:session-cleared", endSession);
  onDeactivated(invalidate);
  onActivated(() => {
    if (!sessionEnded) available.value = true;
  });
  onBeforeUnmount(() => {
    invalidate();
    window.removeEventListener("imgnest:session-cleared", endSession);
  });
  return {
    available,
    capture: () => version,
    isCurrent: (captured: number) => available.value && captured === version,
  };
}
