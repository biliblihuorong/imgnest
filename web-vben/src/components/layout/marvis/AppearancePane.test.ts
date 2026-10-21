import { mount } from "@vue/test-utils";
import { preferences, updatePreferences } from "@vben/preferences";
import { beforeEach, expect, it, vi } from "vitest";
import {
  APPEARANCE_DEFAULTS,
  initShell,
  LEGACY_CLASSIC_DEFAULTS,
  setShell,
  shell,
} from "@/integrations/shell/useShell";
import { LOCALE_MODE_STORAGE_KEY, setLocaleMode } from "@/integrations/locale/useLocaleMode";
import { loadLocaleMessages } from "@vben/locales";
import AppearancePane, { ACCENT_PRESETS, RADIUS_PRESETS } from "./AppearancePane.vue";

vi.mock("@vben/locales", async (original) => ({
  ...(await original<typeof import("@vben/locales")>()),
  loadLocaleMessages: vi.fn().mockResolvedValue(undefined),
}));

// jsdom 没有 matchMedia；Vben 在「跟随系统」模式下会读取它。
vi.stubGlobal(
  "matchMedia",
  vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })),
);

beforeEach(() => {
  localStorage.clear();
  updatePreferences({
    theme: { ...LEGACY_CLASSIC_DEFAULTS, mode: "light" },
    sidebar: { collapsed: false },
  });
  initShell();
  setShell("marvis");
});

function mountPane() {
  const wrapper = mount(AppearancePane);
  const click = (id: string) => wrapper.find(`[data-testid="${id}"]`).trigger("click");
  const pressed = (id: string) => wrapper.find(`[data-testid="${id}"]`).attributes("aria-pressed");
  return { wrapper, click, pressed };
}

it("offers the shared default first and keeps the old classic blue as a preset", () => {
  expect(ACCENT_PRESETS).toHaveLength(5);
  expect(ACCENT_PRESETS[0]).toBe(APPEARANCE_DEFAULTS.colorPrimary);
  expect(ACCENT_PRESETS).toContain(LEGACY_CLASSIC_DEFAULTS.colorPrimary);
  expect(RADIUS_PRESETS).toEqual(["0.25", "0.5", "0.75"]);
});

it("switches to the classic layout and back", async () => {
  const { click, pressed } = mountPane();
  expect(pressed("layout-marvis")).toBe("true");
  await click("layout-classic");
  expect(shell.value).toBe("classic");
  expect(pressed("layout-classic")).toBe("true");
  await click("layout-marvis");
  expect(shell.value).toBe("marvis");
});

it("writes the chosen accent to Vben preferences", async () => {
  const { click, pressed } = mountPane();
  await click("accent-2");
  expect(preferences.theme.colorPrimary).toBe(ACCENT_PRESETS[2]);
  expect(pressed("accent-2")).toBe("true");
  expect(pressed("accent-0")).toBe("false");
});

it.each([
  ["radius-0", "0.25"],
  ["radius-2", "0.75"],
])("writes radius %s", async (id, value) => {
  const { click, pressed } = mountPane();
  await click(id);
  expect(preferences.theme.radius).toBe(value);
  expect(pressed(id)).toBe("true");
});

it.each(["light", "dark", "auto"] as const)("sets the theme mode to %s", async (mode) => {
  const { click, pressed } = mountPane();
  await click(`mode-${mode}`);
  expect(preferences.theme.mode).toBe(mode);
  expect(pressed(`mode-${mode}`)).toBe("true");
});

it("toggles the compact sidebar", async () => {
  const { wrapper, click } = mountPane();
  const toggle = wrapper.find('[data-testid="compact-sidebar"]');
  expect(toggle.attributes("aria-checked")).toBe("false");
  await click("compact-sidebar");
  expect(preferences.sidebar.collapsed).toBe(true);
  expect(toggle.attributes("aria-checked")).toBe("true");
});

it("reset restores the shared defaults", async () => {
  const { click } = mountPane();
  updatePreferences({ theme: { colorPrimary: ACCENT_PRESETS[3], radius: "0.25" } });
  await click("appearance-reset");
  expect(preferences.theme).toMatchObject(APPEARANCE_DEFAULTS);
});

it("keeps a picked accent when the layout is switched from the pane", async () => {
  const { click } = mountPane();
  await click("accent-3");
  await click("layout-classic");
  expect(preferences.theme.colorPrimary).toBe(ACCENT_PRESETS[3]);
});

it("switches the interface language and can follow the browser again", async () => {
  await setLocaleMode("zh-CN");
  const { click, pressed } = mountPane();
  expect(pressed("locale-zh-CN")).toBe("true");
  await click("locale-en-US");
  expect(preferences.app.locale).toBe("en-US");
  expect(loadLocaleMessages).toHaveBeenCalledWith("en-US");
  expect(pressed("locale-en-US")).toBe("true");
  await click("locale-auto");
  expect(pressed("locale-auto")).toBe("true");
  expect(localStorage.getItem(LOCALE_MODE_STORAGE_KEY)).toBe("auto");
  await setLocaleMode("zh-CN");
});
