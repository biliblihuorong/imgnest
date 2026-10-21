import { preferences, updatePreferences } from "@vben/preferences";
import { shallowReadonly, shallowRef } from "vue";

/** 外壳（布局）偏好：marvis 为默认新版，classic 为原 Vben 布局。 */
export type Shell = "marvis" | "classic";

export const SHELL_STORAGE_KEY = "imgnest-shell";

/** 两套外壳共用的默认强调色与圆角；切换布局不改变外观。 */
export const APPEARANCE_DEFAULTS = { colorPrimary: "hsl(230 100% 62%)", radius: "0.75" } as const;

/** Vben 自带、也是 0.1.x 经典布局的默认值；仍停在这里视为用户未自选。 */
export const LEGACY_CLASSIC_DEFAULTS = {
  colorPrimary: "hsl(212 100% 45%)",
  radius: "0.5",
} as const;

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

/** 强调色、圆角各自独立：仍是旧的经典默认值才视为未自选，改为共用默认值。 */
function adoptDefaults(): void {
  const theme: { colorPrimary?: string; radius?: string } = {};
  if (preferences.theme.colorPrimary === LEGACY_CLASSIC_DEFAULTS.colorPrimary)
    theme.colorPrimary = APPEARANCE_DEFAULTS.colorPrimary;
  if (preferences.theme.radius === LEGACY_CLASSIC_DEFAULTS.radius)
    theme.radius = APPEARANCE_DEFAULTS.radius;
  updatePreferences({ theme });
}

/**
 * 读存储 → 写 <html data-shell>。首次启动，或经典布局用户仍停在旧默认值时，
 * 换成共用默认值；新布局里主动选了旧经典蓝的用户保持不变。可重复调用。
 */
export function initShell(): void {
  const stored = readStored();
  apply(stored ?? DEFAULT_SHELL);
  if (stored === "marvis") return;
  adoptDefaults();
  if (!stored) persist(DEFAULT_SHELL);
}

export function setShell(next: Shell): void {
  if (current.value === next) return;
  apply(next);
  persist(next);
  // 外观不随布局变化，但仍要写一次：Vben 每次更新都会替换 preferences.theme，
  // Naive UI 的 token 同步靠它触发，才能重新读取新外壳的底色与边框色。
  updatePreferences({ theme: {} });
}

/** 把强调色与圆角写回共用默认值。 */
export function resetAppearance(): void {
  updatePreferences({ theme: { ...APPEARANCE_DEFAULTS } });
}
