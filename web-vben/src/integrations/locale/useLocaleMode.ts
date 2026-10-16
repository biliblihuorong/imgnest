import { loadLocaleMessages } from "@vben/locales";
import { preferences, updatePreferences } from "@vben/preferences";
import { shallowReadonly, shallowRef, watch } from "vue";

/** 界面语言选择：auto 跟随浏览器语言，其余为固定语言。 */
export type AppLocale = "zh-CN" | "en-US";
export type LocaleMode = "auto" | AppLocale;

export const LOCALE_MODE_STORAGE_KEY = "imgnest-locale-mode";

/** 设置面板与侧栏快捷菜单共用。语言名用各自的写法，不随界面语言翻译；auto 的文案由调用方翻译。 */
export const LOCALE_OPTIONS: { value: LocaleMode; label?: string; lang?: AppLocale }[] = [
  { value: "auto" },
  { value: "zh-CN", label: "简体中文", lang: "zh-CN" },
  { value: "en-US", label: "English", lang: "en-US" },
];

const current = shallowRef<LocaleMode>("auto");
export const localeMode = shallowReadonly(current);

/** 浏览器首选语言里第一个能识别的：中文（含繁体）用简体中文，其他语言用英文。 */
export function detectLocale(languages: readonly string[] = navigator.languages ?? []): AppLocale {
  const list = languages.length > 0 ? languages : [navigator.language ?? ""];
  for (const language of list) {
    const lower = language.toLowerCase();
    if (lower.startsWith("zh")) return "zh-CN";
    if (lower !== "") return "en-US";
  }
  return "zh-CN";
}

function readStored(): LocaleMode | null {
  try {
    const value = localStorage.getItem(LOCALE_MODE_STORAGE_KEY);
    return value === "auto" || value === "zh-CN" || value === "en-US" ? value : null;
  } catch {
    return null;
  }
}

function persist(value: LocaleMode): void {
  try {
    localStorage.setItem(LOCALE_MODE_STORAGE_KEY, value);
  } catch {
    // 存储不可用时只在当前会话内生效。
  }
}

function resolve(mode: LocaleMode): AppLocale {
  return mode === "auto" ? detectLocale() : mode;
}

/** 我们自己写入偏好时置位，用来区分经典布局顶栏语言按钮发起的切换。 */
let applying = false;

async function apply(locale: AppLocale): Promise<void> {
  if (preferences.app.locale === locale) return;
  applying = true;
  try {
    updatePreferences({ app: { locale } });
  } finally {
    applying = false;
  }
  await loadLocaleMessages(locale);
}

let initialized = false;

/**
 * 启动时（i18n 初始化之前）调用：没存过选择时默认跟随浏览器。
 * 之后监听浏览器语言变化；经典布局顶栏的语言按钮直接改偏好，视为用户选了固定语言。
 */
export function initLocaleMode(): void {
  current.value = readStored() ?? "auto";
  const locale = resolve(current.value);
  if (preferences.app.locale !== locale) updatePreferences({ app: { locale } });
  if (initialized) return;
  initialized = true;
  window.addEventListener("languagechange", () => {
    if (current.value === "auto") void apply(detectLocale());
  });
  watch(
    () => preferences.app.locale,
    (locale) => {
      if (applying || resolve(current.value) === locale) return;
      if (locale !== "zh-CN" && locale !== "en-US") return;
      current.value = locale;
      persist(locale);
    },
    { flush: "sync" },
  );
}

export async function setLocaleMode(mode: LocaleMode): Promise<void> {
  current.value = mode;
  persist(mode);
  await apply(resolve(mode));
}
