import { mount } from "@vue/test-utils";
import { preferences, updatePreferences } from "@vben/preferences";
import { beforeEach, expect, it, vi } from "vitest";
import { initShell, setShell, shell, SHELL_DEFAULTS } from "@/integrations/shell/useShell";
import AppearancePane, { ACCENT_PRESETS, RADIUS_PRESETS } from "./AppearancePane.vue";

// jsdom 没有 matchMedia；Vben 在「跟随系统」模式下会读取它。
vi.stubGlobal(
  "matchMedia",
  vi.fn(() => ({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })),
);

beforeEach(() => {
  localStorage.clear();
  updatePreferences({
    theme: { ...SHELL_DEFAULTS.classic, mode: "light" },
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

it("offers both shell defaults among the accent presets", () => {
  expect(ACCENT_PRESETS).toHaveLength(5);
  expect(ACCENT_PRESETS).toContain(SHELL_DEFAULTS.marvis.colorPrimary);
  expect(ACCENT_PRESETS).toContain(SHELL_DEFAULTS.classic.colorPrimary);
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

it("reset restores the current shell defaults", async () => {
  const { click } = mountPane();
  updatePreferences({ theme: { colorPrimary: ACCENT_PRESETS[3], radius: "0.25" } });
  await click("appearance-reset");
  expect(preferences.theme).toMatchObject(SHELL_DEFAULTS.marvis);
});

it("keeps a picked accent when the layout is switched from the pane", async () => {
  const { click } = mountPane();
  await click("accent-3");
  await click("layout-classic");
  expect(preferences.theme.colorPrimary).toBe(ACCENT_PRESETS[3]);
});
