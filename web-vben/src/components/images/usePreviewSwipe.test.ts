import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { NImage, NImageGroup } from "naive-ui";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h } from "vue";
import { usePreviewSwipe } from "./usePreviewSwipe";

type Point = { clientX: number; clientY: number };
let wrappers: VueWrapper[] = [];
let now = 10000;

beforeEach(() => {
  now = 10000;
  vi.spyOn(Date, "now").mockImplementation(() => now);
});

afterEach(() => {
  document.dispatchEvent(new MouseEvent("mouseup"));
  wrappers.forEach((wrapper) => wrapper.unmount());
  wrappers = [];
  vi.restoreAllMocks();
  document.body.innerHTML = "";
});

function mountGroup(urls = ["/one.jpg", "/two.jpg"]) {
  const host = defineComponent({
    setup() {
      const renderToolbar = usePreviewSwipe();
      return () => h(NImageGroup, { renderToolbar }, () => urls.map((src) => h(NImage, { src })));
    },
  });
  const wrapper = mount(host, { attachTo: document.body });
  wrappers.push(wrapper);
  return wrapper;
}

async function openPreview() {
  const wrapper = mountGroup();
  await wrapper.findAll("img")[0]!.trigger("click");
  await flushPromises();
  return preview();
}

function preview(): HTMLImageElement {
  return document.querySelector<HTMLImageElement>(".n-image-preview")!;
}

function touch(type: string, points: Point[], changed = points, target: Element = preview()) {
  const event = new Event(type, { bubbles: true, cancelable: true });
  Object.defineProperties(event, {
    touches: { value: points },
    changedTouches: { value: changed },
  });
  target.dispatchEvent(event);
  return event;
}

const point = (x: number, y = 100): Point => ({ clientX: x, clientY: y });

function wheel(deltaY = -120, target: Element = preview()) {
  target.dispatchEvent(new WheelEvent("wheel", { deltaY, bubbles: true, cancelable: true }));
}

function scale(): number {
  return Number(preview().style.transform.match(/scale\(([^)]+)\)/)?.[1] ?? 1);
}

async function swipe() {
  touch("touchstart", [point(250)]);
  now += 80;
  touch("touchmove", [point(100)]);
  touch("touchend", [], [point(100)]);
  await flushPromises();
}

async function tap(x = 100) {
  touch("touchstart", [point(x)]);
  now += 40;
  touch("touchend", [], [point(x)]);
  await flushPromises();
}

describe("preview gestures with real Naive UI controls", () => {
  it("keeps the current image when a zoomed image is panned", async () => {
    await openPreview();
    wheel();
    expect(scale()).toBeGreaterThan(1);
    await swipe();
    expect(preview().getAttribute("src")).toBe("/one.jpg");
    expect(scale()).toBeGreaterThan(1);
  });

  it("preserves wheel zoom when native mouse dragging starts", async () => {
    const image = await openPreview();
    wheel();
    const zoom = scale();
    expect(zoom).toBeGreaterThan(1);
    image.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, clientX: 150, clientY: 100 }));
    expect(scale()).toBe(zoom);
  });

  it("moves the zoomed image using native Touch coordinates", async () => {
    await openPreview();
    wheel();
    // DOM Touch exposes coordinates as prototype getters rather than own properties.
    touch("touchstart", [Object.create(point(250)) as Point]);
    touch("touchmove", [Object.create(point(100)) as Point]);
    await vi.waitFor(() => expect(preview().style.transform).toContain("translateX(-150px)"));
  });

  it("preserves zoom on unrelated keys and keeps native keyboard zoom working", async () => {
    await openPreview();
    wheel();
    const zoom = scale();
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "a", bubbles: true }));
    expect(scale()).toBe(zoom);
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowUp", bubbles: true }));
    expect(scale()).toBeGreaterThan(zoom);
  });

  it("navigates twice for consecutive fast swipes instead of treating them as taps", async () => {
    await openPreview();
    await swipe();
    expect(preview().getAttribute("src")).toBe("/two.jpg");
    now += 120;
    await swipe();
    expect(preview().getAttribute("src")).toBe("/one.jpg");
    expect(scale()).toBe(1);
  });

  it("zooms on two nearby short taps but not on distant taps", async () => {
    await openPreview();
    await tap(100);
    now += 100;
    await tap(300);
    expect(scale()).toBe(1);
    now += 400;
    await tap(100);
    now += 100;
    await tap(105);
    expect(scale()).toBeGreaterThan(1);
  });

  it("preserves toolbar rotation while wheel zooming", async () => {
    await openPreview();
    const icons = document.querySelectorAll<HTMLElement>(".n-image-preview-toolbar .n-base-icon");
    icons[3]!.click();
    expect(preview().style.transform).toContain("rotate(90deg)");
    wheel();
    expect(preview().style.transform).toContain("rotate(90deg)");
    expect(scale()).toBeGreaterThan(1);
  });

  it("starts the next image at its own scale after toolbar navigation", async () => {
    await openPreview();
    wheel();
    const firstZoom = scale();
    wheel();
    document.querySelectorAll<HTMLElement>(".n-image-preview-toolbar .n-base-icon")[1]!.click();
    await flushPromises();
    expect(preview().getAttribute("src")).toBe("/two.jpg");
    expect(scale()).toBe(1);
    wheel();
    expect(scale()).toBe(firstZoom);
  });

  it("zooms with a pinch and allows the remaining finger to pan without navigation", async () => {
    await openPreview();
    touch("touchstart", [point(100), point(200)]);
    touch("touchmove", [point(50), point(250)]);
    expect(scale()).toBeGreaterThan(1);
    touch("touchend", [point(50)], [point(250)]);
    now += 80;
    touch("touchmove", [point(200)]);
    touch("touchend", [], [point(200)]);
    await flushPromises();
    expect(preview().getAttribute("src")).toBe("/one.jpg");
    expect(scale()).toBeGreaterThan(1);
  });

  it("ignores toolbar touch gestures", async () => {
    await openPreview();
    const toolbar = document.querySelector(".n-image-preview-toolbar")!;
    touch("touchstart", [point(250)], undefined, toolbar);
    now += 80;
    touch("touchend", [], [point(100)], toolbar);
    await flushPromises();
    expect(preview().getAttribute("src")).toBe("/one.jpg");
    expect(scale()).toBe(1);
  });

  it("preserves backdrop tap-to-close after zooming", async () => {
    await openPreview();
    wheel();
    const backdrop = document.querySelector<HTMLElement>(".n-image-preview-overlay")!;
    const start = touch("touchstart", [point(15, 30)], undefined, backdrop);
    now += 40;
    const end = touch("touchend", [], [point(15, 30)], backdrop);
    expect(start.defaultPrevented).toBe(false);
    expect(end.defaultPrevented).toBe(false);
    backdrop.click();
    await flushPromises();
    expect(document.querySelector(".n-image-preview-overlay")).toBeNull();
  });

  it("cancels an interrupted swipe and still accepts the next complete gesture", async () => {
    await openPreview();
    touch("touchstart", [point(250)]);
    touch("touchcancel", [], [point(100)]);
    now += 80;
    touch("touchend", [], [point(100)]);
    await flushPromises();
    expect(preview().getAttribute("src")).toBe("/one.jpg");
    now += 400;
    await swipe();
    expect(preview().getAttribute("src")).toBe("/two.jpg");
  });

  it("keeps gestures attached while another consumer remains mounted", async () => {
    await openPreview();
    const second = mountGroup(["/later.jpg"]);
    wrappers[0]!.unmount();
    wrappers.shift();
    await second.find("img").trigger("click");
    await flushPromises();
    wheel();
    expect(scale()).toBeGreaterThan(1);
    expect(preview().getAttribute("src")).toBe("/later.jpg");
  });
});
