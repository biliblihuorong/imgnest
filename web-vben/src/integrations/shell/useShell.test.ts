import { preferences, updatePreferences } from "@vben/preferences";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  APPEARANCE_DEFAULTS,
  initShell,
  LEGACY_CLASSIC_DEFAULTS,
  resetAppearance,
  setShell,
  shell,
  SHELL_STORAGE_KEY,
} from "./useShell";

beforeEach(() => {
  localStorage.clear();
  // Vben 自带的默认值，就是经典布局过去的默认值。
  updatePreferences({ theme: { ...LEGACY_CLASSIC_DEFAULTS } });
  delete document.documentElement.dataset.shell;
});
afterEach(() => {
  vi.restoreAllMocks();
});

it("defaults to marvis and marks <html>", () => {
  initShell();
  expect(shell.value).toBe("marvis");
  expect(document.documentElement.dataset.shell).toBe("marvis");
});

it("adopts the shared defaults when the user never chose a colour", () => {
  initShell();
  expect(preferences.theme).toMatchObject(APPEARANCE_DEFAULTS);
});

it("restores a stored classic choice with the same defaults as marvis", () => {
  localStorage.setItem(SHELL_STORAGE_KEY, "classic");
  initShell();
  expect(shell.value).toBe("classic");
  expect(document.documentElement.dataset.shell).toBe("classic");
  expect(preferences.theme).toMatchObject(APPEARANCE_DEFAULTS);
});

it.each(["", "vben", "null"])("falls back to marvis for stored %j", (value) => {
  localStorage.setItem(SHELL_STORAGE_KEY, value);
  initShell();
  expect(shell.value).toBe("marvis");
});

it("survives a throwing storage", () => {
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
    throw new Error("blocked");
  });
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("blocked");
  });
  expect(() => initShell()).not.toThrow();
  expect(shell.value).toBe("marvis");
  expect(() => setShell("classic")).not.toThrow();
  expect(shell.value).toBe("classic");
});

it("setShell persists and keeps the accent and radius unchanged", () => {
  initShell();
  setShell("classic");
  expect(localStorage.getItem(SHELL_STORAGE_KEY)).toBe("classic");
  expect(document.documentElement.dataset.shell).toBe("classic");
  expect(preferences.theme).toMatchObject(APPEARANCE_DEFAULTS);
  setShell("marvis");
  expect(preferences.theme).toMatchObject(APPEARANCE_DEFAULTS);
});

it("keeps a colour and radius the user picked across switches", () => {
  initShell();
  updatePreferences({ theme: { colorPrimary: "hsl(160 84% 34%)", radius: "0.25" } });
  setShell("classic");
  expect(preferences.theme).toMatchObject({ colorPrimary: "hsl(160 84% 34%)", radius: "0.25" });
});

it("keeps the old classic blue when a marvis user picked it explicitly", () => {
  initShell();
  updatePreferences({ theme: { colorPrimary: LEGACY_CLASSIC_DEFAULTS.colorPrimary } });
  initShell();
  expect(preferences.theme.colorPrimary).toBe(LEGACY_CLASSIC_DEFAULTS.colorPrimary);
});

it("keeps classic choices matching the old defaults after the initial migration", () => {
  localStorage.setItem(SHELL_STORAGE_KEY, "classic");
  initShell();
  expect(preferences.theme).toMatchObject(APPEARANCE_DEFAULTS);

  updatePreferences({ theme: { colorPrimary: "hsl(212 100% 45%)", radius: "0.5" } });
  initShell();
  expect(preferences.theme).toMatchObject({ colorPrimary: "hsl(212 100% 45%)", radius: "0.5" });
});

it("keeps a marvis user's old-blue and medium-radius choices after switching to classic and reloading", () => {
  localStorage.setItem(SHELL_STORAGE_KEY, "marvis");
  initShell();
  updatePreferences({ theme: { colorPrimary: "hsl(212 100% 45%)", radius: "0.5" } });
  setShell("classic");

  initShell();
  expect(shell.value).toBe("classic");
  expect(preferences.theme).toMatchObject({ colorPrimary: "hsl(212 100% 45%)", radius: "0.5" });
});

it("preserves the current appearance when a migration cannot be recorded", () => {
  localStorage.setItem(SHELL_STORAGE_KEY, "classic");
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new DOMException("quota", "QuotaExceededError");
  });

  expect(() => initShell()).not.toThrow();
  expect(preferences.theme).toMatchObject({ colorPrimary: "hsl(212 100% 45%)", radius: "0.5" });
});

it("resetAppearance returns to the shared defaults in either shell", () => {
  localStorage.setItem(SHELL_STORAGE_KEY, "classic");
  initShell();
  updatePreferences({ theme: { colorPrimary: "hsl(160 84% 34%)", radius: "0.25" } });
  resetAppearance();
  expect(preferences.theme).toMatchObject(APPEARANCE_DEFAULTS);
});

it("always replaces preferences.theme on a switch so Naive UI re-reads the shell's tokens", () => {
  initShell();
  const before = preferences.theme;
  setShell("classic");
  expect(preferences.theme).not.toBe(before);
});
