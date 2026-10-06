import { preferences, updatePreferences } from "@vben/preferences";
import { shallowReadonly, shallowRef } from "vue";

/** 外壳（布局）偏好：marvis 为默认新版，classic 为原 Vben 布局。 */
export type Shell = "marvis" | "classic";

export const SHELL_STORAGE_KEY = "imgnest-shell";

/** 每套外壳自己的默认强调色与圆角；用户没自选时跟随外壳。 */
export const SHELL_DEFAULTS: Record<Shell, { colorPrimary: string; radius: string }> = {
  classic: { colorPrimary: "hsl(212 100% 45%)", radius: "0.5" },
  marvis: { colorPrimary: "hsl(230 100% 62%)", radius: "0.75" },
};

const DEFAULT_SHELL: Shell = "marvis";
const current = shallowRef<Shell>(DEFAULT_SHELL);

export const shell = shallowReadonly(current);

function readStored(): Shell | null {
  try {
    const value = localStorage.getItem(SHELL_STORAGE_KEY);
    return value === "marvis" || value === "classic" ? value : null;
  } catch {
    return null;
  }
}

function persist(value: Shell): void {
  try {
    localStorage.setItem(SHELL_STORAGE_KEY, value);
  } catch {
    // 存储不可用时只在当前会话内生效。
  }
}

function apply(value: Shell): void {
  current.value = value;
  document.documentElement.dataset.shell = value;
}

/** 强调色、圆角各自独立：仍等于 from 的默认值才视为未自选，改写为 to 的默认值。 */
function followShell(from: Shell, to: Shell): void {
  const theme: { colorPrimary?: string; radius?: string } = {};
  if (preferences.theme.colorPrimary === SHELL_DEFAULTS[from].colorPrimary)
    theme.colorPrimary = SHELL_DEFAULTS[to].colorPrimary;
  if (preferences.theme.radius === SHELL_DEFAULTS[from].radius)
    theme.radius = SHELL_DEFAULTS[to].radius;
  // 即使两项都是用户自选（theme 为空）也要写一次：Vben 每次更新都会替换 preferences.theme，
  // Naive UI 的 token 同步靠它触发，才能重新读取新外壳的底色与边框色。
  updatePreferences({ theme });
}

/** 读存储 → 写 <html data-shell>；首次启动时把未自选的外观对齐到默认外壳。可重复调用。 */
export function initShell(): void {
  const stored = readStored();
  if (stored) {
    apply(stored);
    return;
  }
  apply(DEFAULT_SHELL);
  followShell("classic", DEFAULT_SHELL);
  persist(DEFAULT_SHELL);
}

export function setShell(next: Shell): void {
  const previous = current.value;
  if (previous === next) return;
  apply(next);
  followShell(previous, next);
  persist(next);
}

/** 把强调色与圆角写回当前外壳默认值，回到「跟随布局」状态。 */
export function resetAppearance(): void {
  updatePreferences({ theme: { ...SHELL_DEFAULTS[current.value] } });
}
