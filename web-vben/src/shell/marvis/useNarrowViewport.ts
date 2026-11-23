import { useMediaQuery } from "@vueuse/core";
import type { Ref } from "vue";

/** 视口宽度小于 px 时为 true。 */
export function useViewportBelow(px: number): Readonly<Ref<boolean>> {
  return useMediaQuery(`(max-width: ${px - 1}px)`);
}

/** 视口宽度不小于 px 时为 true；不支持 matchMedia 的环境按 false 处理。 */
export function useViewportAtLeast(px: number): Readonly<Ref<boolean>> {
  return useMediaQuery(`(min-width: ${px}px)`);
}
