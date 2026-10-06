import { preferences, updatePreferences } from "@vben/preferences";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  initShell,
  resetAppearance,
  setShell,
  shell,
  SHELL_DEFAULTS,
  SHELL_STORAGE_KEY,
} from "./useShell";

beforeEach(() => {
  localStorage.clear();
  updatePreferences({ theme: { ...SHELL_DEFAULTS.classic } });
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

it("adopts marvis defaults when the user never chose a colour", () => {
  initShell();
  expect(preferences.theme.colorPrimary).toBe(SHELL_DEFAULTS.marvis.colorPrimary);
  expect(preferences.theme.radius).toBe("0.75");
});

it("restores a stored classic choice", () => {
  localStorage.setItem(SHELL_STORAGE_KEY, "classic");
  initShell();
  expect(shell.value).toBe("classic");
  expect(document.documentElement.dataset.shell).toBe("classic");
  expect(preferences.theme.colorPrimary).toBe(SHELL_DEFAULTS.classic.colorPrimary);
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

it("setShell persists and swaps untouched defaults", () => {
  initShell();
  setShell("classic");
  expect(localStorage.getItem(SHELL_STORAGE_KEY)).toBe("classic");
  expect(document.documentElement.dataset.shell).toBe("classic");
  expect(preferences.theme.colorPrimary).toBe(SHELL_DEFAULTS.classic.colorPrimary);
  expect(preferences.theme.radius).toBe("0.5");
});

it("keeps a colour the user picked, still follows radius", () => {
  initShell();
  updatePreferences({ theme: { colorPrimary: "hsl(160 84% 34%)" } });
  setShell("classic");
  expect(preferences.theme.colorPrimary).toBe("hsl(160 84% 34%)");
  expect(preferences.theme.radius).toBe("0.5");
});

it("keeps the other shell's default colour once the user picked it explicitly", () => {
  initShell();
  updatePreferences({ theme: { colorPrimary: SHELL_DEFAULTS.classic.colorPrimary } });
  initShell();
  expect(preferences.theme.colorPrimary).toBe(SHELL_DEFAULTS.classic.colorPrimary);
});

it("resetAppearance returns to the current shell defaults", () => {
  initShell();
  updatePreferences({ theme: { colorPrimary: "hsl(160 84% 34%)", radius: "0.25" } });
  resetAppearance();
  expect(preferences.theme).toMatchObject(SHELL_DEFAULTS.marvis);
});
