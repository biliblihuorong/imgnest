import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { createPinia } from "pinia";
import { defineComponent, h, ref } from "vue";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useAuthStore } from "@/stores/auth";
import MarvisCommandPalette from "./MarvisCommandPalette.vue";
import { shellTestRouter, testUser } from "./testing";

vi.mock("@/api/albums", () => ({
  suggestAlbums: vi.fn().mockResolvedValue({ items: [], hasMore: false }),
}));

let wrapper: VueWrapper | undefined;
beforeEach(() => {
  localStorage.clear();
});
afterEach(() => {
  wrapper?.unmount();
  wrapper = undefined;
  document.body.innerHTML = "";
});

/** 弹窗内容 teleport 到 body，统一做 body 级查询。 */
const input = () => document.body.querySelector<HTMLInputElement>('[data-testid="palette-input"]');
const isOpen = () => input() !== null;
const closed = () => expect(isOpen()).toBe(false);

async function mountPalette(options: { anonymous?: boolean } = {}) {
  const pinia = createPinia();
  const auth = useAuthStore(pinia);
  if (!options.anonymous) {
    auth.token = "token";
    auth.user = testUser("user");
  }
  const router = shellTestRouter();
  await router.push("/upload");
  const show = ref(false);
  const Host = defineComponent({
    setup: () => () => [
      h("button", { "data-testid": "trigger", onClick: () => (show.value = true) }, "open"),
      h(MarvisCommandPalette, {
        show: show.value,
        "onUpdate:show": (value: boolean) => (show.value = value),
      }),
    ],
  });
  wrapper = mount(Host, { attachTo: document.body, global: { plugins: [pinia, router] } });
  await flushPromises();
  return { router, show };
}

async function press(target: EventTarget, init: KeyboardEventInit) {
  target.dispatchEvent(new KeyboardEvent("keydown", { bubbles: true, cancelable: true, ...init }));
  await flushPromises();
}
async function type(value: string) {
  const el = input()!;
  el.value = value;
  el.setSelectionRange(value.length, value.length);
  el.dispatchEvent(new Event("input", { bubbles: true }));
  await flushPromises();
}

it.each([{ ctrlKey: true }, { metaKey: true }])(
  "opens on %o + K and closes on Escape",
  async (mod) => {
    await mountPalette();
    await press(window, { key: "k", ...mod });
    expect(isOpen()).toBe(true);
    await press(input()!, { key: "Escape" });
    closed();
  },
);

it("ignores the shortcut for anonymous visitors", async () => {
  await mountPalette({ anonymous: true });
  await press(window, { key: "k", ctrlKey: true });
  expect(isOpen()).toBe(false);
});

it("submits the raw query to /images using the unified-search route params", async () => {
  const { router } = await mountPalette();
  await press(window, { key: "k", ctrlKey: true });
  await type("  sunset beach ");
  await press(input()!, { key: "Enter" });
  expect(router.currentRoute.value.path).toBe("/images");
  expect(router.currentRoute.value.query).toMatchObject({
    qv: "1",
    q: "sunset beach",
    page: "1",
    size: "20",
  });
  expect(typeof router.currentRoute.value.query.tz).toBe("string");
  closed();
});

it("opens the image library without a query when the input is empty", async () => {
  const { router } = await mountPalette();
  await press(window, { key: "k", ctrlKey: true });
  await press(input()!, { key: "Enter" });
  expect(router.currentRoute.value.fullPath).toBe("/images");
});

it("does not submit while an IME composition is active", async () => {
  const { router } = await mountPalette();
  await press(window, { key: "k", ctrlKey: true });
  input()!.dispatchEvent(new CompositionEvent("compositionstart", { bubbles: true }));
  await type("ri");
  await press(input()!, { key: "Enter", isComposing: true });
  expect(router.currentRoute.value.path).toBe("/upload");
  expect(isOpen()).toBe(true);
});

it("applies a chosen suggestion to the input instead of navigating", async () => {
  const { router } = await mountPalette();
  await press(window, { key: "k", ctrlKey: true });
  input()!.dispatchEvent(new Event("focus"));
  await type("form");
  await press(input()!, { key: "ArrowDown" });
  await press(input()!, { key: "Enter" });
  expect(input()!.value).toBe("format:");
  expect(router.currentRoute.value.path).toBe("/upload");
});

it("navigates to a matching page entry", async () => {
  const { router } = await mountPalette();
  await press(window, { key: "k", ctrlKey: true });
  await type("相册");
  const entries = [...document.body.querySelectorAll<HTMLElement>('[data-testid="palette-page"]')];
  expect(entries.map((entry) => entry.textContent?.trim())).toEqual(["我的相册"]);
  entries[0]!.click();
  await flushPromises();
  expect(router.currentRoute.value.path).toBe("/albums");
  closed();
});

it("returns focus to the trigger after closing", async () => {
  await mountPalette();
  const trigger = document.body.querySelector<HTMLElement>('[data-testid="trigger"]')!;
  trigger.focus();
  trigger.click();
  await flushPromises();
  expect(document.activeElement).toBe(input());
  await press(input()!, { key: "Escape" });
  closed();
  expect(document.activeElement).toBe(trigger);
});

it("starts empty every time it opens", async () => {
  await mountPalette();
  await press(window, { key: "k", ctrlKey: true });
  await type("leftover");
  await press(input()!, { key: "Escape" });
  closed();
  await press(window, { key: "k", ctrlKey: true });
  expect(input()!.value).toBe("");
});

it("closes when the backdrop is pressed and keeps Tab inside the dialog", async () => {
  await mountPalette();
  await press(window, { key: "k", ctrlKey: true });
  const buttons = [...document.body.querySelectorAll<HTMLElement>(".mv-palette button")];
  const last = buttons.at(-1)!;
  last.focus();
  await press(last, { key: "Tab" });
  expect(document.activeElement).toBe(input());
  await press(input()!, { key: "Tab", shiftKey: true });
  expect(document.activeElement).toBe(last);
  document.body
    .querySelector(".mv-dialog-overlay")!
    .dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
  await flushPromises();
  closed();
});

it("closes on Escape even when focus has left the dialog", async () => {
  await mountPalette();
  await press(window, { key: "k", ctrlKey: true });
  input()!.blur();
  expect(document.activeElement).toBe(document.body);
  await press(document.body, { key: "Escape" });
  closed();
});

it("does not open over another modal dialog", async () => {
  await mountPalette();
  const other = document.createElement("div");
  other.setAttribute("role", "dialog");
  other.setAttribute("aria-modal", "true");
  document.body.appendChild(other);
  await press(window, { key: "k", ctrlKey: true });
  expect(isOpen()).toBe(false);
  other.remove();
  await press(window, { key: "k", ctrlKey: true });
  expect(isOpen()).toBe(true);
});

it("ignores the shortcut during IME composition", async () => {
  await mountPalette();
  await press(window, { key: "k", ctrlKey: true, isComposing: true });
  expect(isOpen()).toBe(false);
});
