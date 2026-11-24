import { preferences, updatePreferences } from "@vben/preferences";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  detectLocale,
  initLocaleMode,
  LOCALE_MODE_STORAGE_KEY,
  localeMode,
  setLocaleMode,
} from "./useLocaleMode";

vi.mock("@vben/locales", async (original) => ({
  ...(await original<typeof import("@vben/locales")>()),
  loadLocaleMessages: vi.fn().mockResolvedValue(undefined),
}));

function browserLanguages(languages: string[]): void {
  vi.spyOn(navigator, "languages", "get").mockReturnValue(languages);
}

beforeEach(() => {
  localStorage.clear();
  updatePreferences({ app: { locale: "zh-CN" } });
});
afterEach(() => {
  vi.restoreAllMocks();
});

it("maps any Chinese browser language to zh-CN and everything else to en-US", () => {
  expect(detectLocale(["zh-TW", "en"])).toBe("zh-CN");
  expect(detectLocale(["en-GB", "zh-CN"])).toBe("en-US");
  expect(detectLocale(["ja-JP"])).toBe("en-US");
});

it("follows the browser language when nothing was chosen yet", () => {
  browserLanguages(["en-US"]);
  initLocaleMode();
  expect(localeMode.value).toBe("auto");
  expect(preferences.app.locale).toBe("en-US");
});

it("keeps a stored fixed language regardless of the browser", () => {
  browserLanguages(["en-US"]);
  localStorage.setItem(LOCALE_MODE_STORAGE_KEY, "zh-CN");
  initLocaleMode();
  expect(localeMode.value).toBe("zh-CN");
  expect(preferences.app.locale).toBe("zh-CN");
});

it("treats a language picked from the classic header as a fixed choice", async () => {
  browserLanguages(["zh-CN"]);
  await setLocaleMode("auto");
  initLocaleMode();
  updatePreferences({ app: { locale: "en-US" } });
  expect(localeMode.value).toBe("en-US");
  expect(localStorage.getItem(LOCALE_MODE_STORAGE_KEY)).toBe("en-US");
});
