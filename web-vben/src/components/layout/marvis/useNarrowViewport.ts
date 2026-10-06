import { useMediaQuery } from "@vueuse/core";
import type { Ref } from "vue";

/** 视口宽度小于 px 时为 true。 */
export function useViewportBelow(px: number): Readonly<Ref<boolean>> {
  return useMediaQuery(`(max-width: ${px - 1}px)`);
}
