import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { i18n } from "@vben/locales";
import UnifiedImageSearch from "./UnifiedImageSearch.vue";
import { suggestAlbums } from "@/api/albums";
vi.mock("@/api/albums", () => ({ suggestAlbums: vi.fn() }));
enableAutoUnmount(afterEach);
const create = (raw = "") =>
  mount(UnifiedImageSearch, {
    props: {
      modelValue: raw,
      timezone: "UTC",
      authorizedAlbums: [],
      "onUpdate:modelValue": (value: string) => wrapper?.setProps({ modelValue: value }),
    },
    attachTo: document.body,
  });
let wrapper: ReturnType<typeof create>;
afterEach(() => {
  vi.useRealTimers();
  vi.resetAllMocks();
});
describe("unified search input", () => {
  it("keeps IME Enter local and submits only after composition ends", async () => {
    wrapper = create();
    const input = wrapper.get("input");
    await input.trigger("compositionstart");
    await input.setValue("假");
    await input.trigger("keydown", { key: "Enter", isComposing: true });
    expect(wrapper.emitted("submit")).toBeUndefined();
    await input.trigger("compositionend");
    await input.trigger("keydown", { key: "Enter" });
    expect(wrapper.emitted("submit")).toHaveLength(1);
  });
  it("candidate Enter inserts without submitting; second Enter submits", async () => {
    wrapper = create("format:j");
    const input = wrapper.get("input");
    await input.trigger("focus");
    input.element.setSelectionRange(8, 8);
    await input.trigger("click");
    await input.trigger("keydown", { key: "ArrowDown" });
    await input.trigger("keydown", { key: "Enter" });
    await flushPromises();
    expect(wrapper.props("modelValue")).toBe("format:jpg");
    expect(wrapper.emitted("submit")).toBeUndefined();
    await input.trigger("keydown", { key: "Enter" });
    expect(wrapper.emitted("submit")).toHaveLength(1);
  });
  it("removing a merged chip changes the same draft and preserves other conditions", async () => {
    wrapper = create("format:jpg extension:png camera:canon");
    await wrapper.get('[data-remove="format"]').trigger("click");
    expect(wrapper.props("modelValue")).toBe("camera:canon");
    expect(wrapper.emitted("submit")).toBeUndefined();
  });
  it("help does not submit or overwrite nonempty draft without explicit confirmation", async () => {
    wrapper = create("camera:canon");
    await wrapper.get('[data-testid="search-help"]').trigger("click");
    await flushPromises();
    expect(document.body.textContent).toContain("UTC");
    const example = [...document.body.querySelectorAll("button")].find(
      (b) => b.dataset.example === "0",
    )!;
    example.click();
    await flushPromises();
    expect(wrapper.props("modelValue")).toBe("camera:canon");
    const confirm = document.body.querySelector<HTMLButtonElement>("[data-confirm-example]")!;
    confirm.click();
    await flushPromises();
    expect(wrapper.props("modelValue")).not.toBe("camera:canon");
    expect(wrapper.emitted("submit")).toBeUndefined();
  });
  it("locale switching preserves raw syntax", async () => {
    wrapper = create("album:#42");
    i18n.global.locale.value = "en-US";
    await flushPromises();
    expect(wrapper.props("modelValue")).toBe("album:#42");
    expect(wrapper.text()).toContain("Search");
  });
  it("album suggestions debounce and send fixed scope", async () => {
    vi.useFakeTimers();
    vi.mocked(suggestAlbums).mockResolvedValue({
      items: [{ id: "42", name: "旅行" }],
      hasMore: false,
    });
    wrapper = create("album:");
    await wrapper.setProps({ lockedAlbumId: 42 });
    const input = wrapper.get("input");
    input.element.setSelectionRange(6, 6);
    await input.trigger("focus");
    await input.trigger("click");
    await vi.advanceTimersByTimeAsync(249);
    expect(suggestAlbums).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    await flushPromises();
    expect(suggestAlbums).toHaveBeenCalledWith("", 1, expect.any(AbortSignal), "42");
  });
});

it("Tab and empty Backspace preserve draft while Escape closes suggestions", async () => {
  wrapper = create("format:j");
  const input = wrapper.get("input");
  input.element.setSelectionRange(8, 8);
  await input.trigger("focus");
  await input.trigger("click");
  await input.trigger("keydown", { key: "ArrowDown" });
  expect(wrapper.get('[role="combobox"]').attributes("aria-expanded")).toBe("true");
  await input.trigger("keydown", { key: "Tab" });
  expect(wrapper.props("modelValue")).toBe("format:j");
  expect(wrapper.emitted("submit")).toBeUndefined();
  await input.trigger("keydown", { key: "Escape" });
  expect(wrapper.get('[role="combobox"]').attributes("aria-expanded")).toBe("false");
  await wrapper.setProps({ modelValue: "" });
  await input.trigger("keydown", { key: "Backspace" });
  expect(wrapper.props("modelValue")).toBe("");
});
it("locates errors using original UTF16 selection ranges", async () => {
  wrapper = create("😀 foo:bar");
  const locate = wrapper.findAll("button").find((b) => b.text() === "定位")!;
  await locate.trigger("click");
  await flushPromises();
  const input = wrapper.get("input").element;
  expect(input.selectionStart).toBe(3);
  expect(input.selectionEnd).toBe(6);
});
it("merged aliases expose one chip and selecting the chip edits the original token", async () => {
  wrapper = create("extension:jpeg format:png");
  expect(wrapper.findAll('[data-remove="format"]')).toHaveLength(1);
  await wrapper.get(".search-chip__edit").trigger("click");
  await flushPromises();
  expect(wrapper.get("input").element.selectionStart).toBe(10);
  expect(wrapper.get("input").element.selectionEnd).toBe(14);
});
it("opening and closing help preserves draft and returns keyboard focus", async () => {
  wrapper = create("camera:canon");
  const trigger = wrapper.get('[data-testid="search-help"]');
  await trigger.trigger("focus");
  await flushPromises();
  expect(document.body.textContent).toContain("可用关键词和条件搜索");
  await trigger.trigger("click");
  await flushPromises();
  expect(trigger.attributes("aria-expanded")).toBe("true");
  expect(document.body.querySelector('[role="dialog"]')).not.toBeNull();
  const close = [...document.body.querySelectorAll("button")].find(
    (b) => b.textContent?.trim() === "关闭帮助",
  )!;
  close.click();
  await flushPromises();
  expect(wrapper.props("modelValue")).toBe("camera:canon");
  expect(wrapper.emitted("submit")).toBeUndefined();
});

it("labels the image display zone when a shared query uses a different zone", async () => {
  wrapper = create("after:2026-10-01");
  await wrapper.setProps({ timezone: "America/New_York" });
  expect(wrapper.text()).toContain("日期时区：America/New_York");
  expect(wrapper.text()).toContain(
    `图片时间显示：${Intl.DateTimeFormat().resolvedOptions().timeZone}`,
  );
});
