import type { ImageRenderToolbar, ImageRenderToolbarProps } from "naive-ui";
import { h, onBeforeUnmount, onMounted } from "vue";

type Action = keyof ImageRenderToolbarProps["nodes"];
type Point = { clientX: number; clientY: number };
type Gesture = {
  kind: "swipe" | "pan" | "pinch";
  image: HTMLImageElement;
  start: Point;
  last: Point;
  time: number;
  moved: boolean;
  distance: number;
};

// Public toolbar nodes retain Naive UI's handlers and its single transform state.
const renderToolbar: ImageRenderToolbar = ({ nodes }) =>
  Object.entries(nodes).map(([action, node]) =>
    h("span", { "data-preview-action": action, style: { display: "inline-flex" } }, [node]),
  );

let consumers = 0;
let stopObserving: (() => void) | undefined;

/** Bind the returned toolbar renderer to NImageGroup alongside its gesture support. */
export function usePreviewSwipe(): ImageRenderToolbar {
  let mounted = false;
  onMounted(() => {
    mounted = true;
    if (consumers++ === 0) stopObserving = observePreviews();
  });
  onBeforeUnmount(() => {
    if (!mounted) return;
    mounted = false;
    if (--consumers === 0) {
      stopObserving?.();
      stopObserving = undefined;
    }
  });
  return renderToolbar;
}

function observePreviews(): () => void {
  const attached = new Map<HTMLElement, { image: Element | null; dispose: () => void }>();
  const sync = () => {
    for (const [container, binding] of attached) {
      if (!container.isConnected || binding.image !== container.querySelector(".n-image-preview")) {
        binding.dispose();
        attached.delete(container);
      }
    }
    for (const container of document.querySelectorAll<HTMLElement>(".n-image-preview-container")) {
      if (!attached.has(container)) {
        attached.set(container, {
          image: container.querySelector(".n-image-preview"),
          dispose: attachGestures(container),
        });
      }
    }
  };
  const observer = new MutationObserver(sync);
  observer.observe(document.body, { childList: true, subtree: true });
  sync();
  return () => {
    observer.disconnect();
    for (const binding of attached.values()) binding.dispose();
    attached.clear();
  };
}

function attachGestures(container: HTMLElement): () => void {
  let gesture: Gesture | undefined;
  let lastTap: { point: Point; time: number } | undefined;
  let wheelDelta = 0;
  const image = () => container.querySelector<HTMLImageElement>(".n-image-preview");
  const scale = () => Number(image()?.style.transform.match(/scale\(([^)]+)\)/)?.[1] ?? 1);
  const distance = (a: Point, b: Point) => Math.hypot(a.clientX - b.clientX, a.clientY - b.clientY);
  const isToolbar = (target: EventTarget | null) =>
    target instanceof Element && target.closest(".n-image-preview-toolbar") !== null;

  function action(name: Action): boolean {
    const icon = container.querySelector<HTMLElement>(
      `[data-preview-action="${name}"] .n-base-icon`,
    );
    if (!icon) return false;
    icon.click();
    return true;
  }

  function key(name: string): void {
    document.dispatchEvent(new KeyboardEvent("keydown", { key: name, bubbles: true }));
  }

  function zoom(inward: boolean): void {
    // Unmarked standalone NImage previews can still use their native keyboard controls.
    if (!action(inward ? "zoomIn" : "zoomOut")) key(inward ? "ArrowUp" : "ArrowDown");
  }

  function mouse(type: string, target: EventTarget, point: Point): void {
    target.dispatchEvent(
      new MouseEvent(type, {
        clientX: point.clientX,
        clientY: point.clientY,
        bubbles: true,
        button: 0,
      }),
    );
  }

  function finishPan(): void {
    if (gesture?.kind === "pan") mouse("mouseup", document, gesture.last);
  }

  function cancel(): void {
    finishPan();
    gesture = undefined;
    lastTap = undefined;
  }

  function navigate(forward: boolean): void {
    cancel();
    if (!action(forward ? "next" : "prev")) key(forward ? "ArrowRight" : "ArrowLeft");
  }

  function onKeydown(event: KeyboardEvent): void {
    if (!container.querySelector(".n-image-preview-overlay")) return;
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    // The native toolbar also resets its transform before changing images.
    const name = event.key === "ArrowRight" ? "next" : "prev";
    if (!container.querySelector(`[data-preview-action="${name}"]`)) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    navigate(event.key === "ArrowRight");
  }

  function onTouchStart(event: TouchEvent): void {
    finishPan();
    gesture = undefined;
    const img = image();
    const first = event.touches[0];
    if (!img || !first || isToolbar(event.target)) {
      lastTap = undefined;
      return;
    }
    if (event.target !== img) lastTap = undefined;
    if (event.touches.length > 2) {
      lastTap = undefined;
      return;
    }
    const pinching = event.touches.length === 2;
    gesture = {
      kind: pinching ? "pinch" : scale() > 1 ? "pan" : "swipe",
      image: img,
      start: first,
      last: first,
      time: Date.now(),
      moved: pinching,
      distance: pinching ? distance(first, event.touches[1]!) : 0,
    };
    if (pinching) lastTap = undefined;
    if (gesture.kind === "pan") mouse("mousedown", img, first);
    // A short backdrop tap must still produce Naive UI's click-to-close.
    if (pinching || (gesture.kind === "pan" && event.target === img)) event.preventDefault();
  }

  function onTouchMove(event: TouchEvent): void {
    if (!gesture) return;
    const first = event.touches[0];
    if (!first || event.touches.length > 2) {
      cancel();
      return;
    }
    event.preventDefault();
    gesture.last = first;
    gesture.moved ||= distance(first, gesture.start) > 10;
    if (gesture.kind === "pinch" && event.touches.length === 2) {
      const current = distance(first, event.touches[1]!);
      const ratio = current / gesture.distance;
      if (ratio >= 1.2 || ratio <= 1 / 1.2) {
        zoom(ratio > 1);
        gesture.distance = current;
      }
    } else if (gesture.kind === "pan") {
      mouse("mousemove", document, first);
    }
  }

  function onTouchEnd(event: TouchEvent): void {
    if (!gesture) return;
    if (event.touches.length > 0) {
      if (gesture.kind === "pinch" && event.touches.length === 1 && scale() > 1) {
        gesture.kind = "pan";
        gesture.last = event.touches[0]!;
        mouse("mousedown", gesture.image, gesture.last);
      }
      return;
    }
    const completed = gesture;
    finishPan();
    gesture = undefined;
    const end = event.changedTouches[0];
    if (!end || completed.kind === "pinch") return;
    const dx = end.clientX - completed.start.clientX;
    const dy = end.clientY - completed.start.clientY;
    const elapsed = Math.max(1, Date.now() - completed.time);
    if (completed.moved || distance(end, completed.start) > 10) {
      lastTap = undefined;
      if (
        completed.kind === "swipe" &&
        Math.abs(dx) >= 50 &&
        Math.abs(dx) / elapsed >= 0.3 &&
        Math.abs(dy) <= Math.abs(dx) * 0.7
      ) {
        event.preventDefault();
        navigate(dx < 0);
      }
      return;
    }
    if (elapsed > 250) {
      lastTap = undefined;
      return;
    }
    if (lastTap && Date.now() - lastTap.time < 300 && distance(lastTap.point, end) <= 24) {
      event.preventDefault();
      if (scale() > 1) {
        for (let i = 0; i < 20 && scale() > 1; i++) zoom(false);
      } else {
        zoom(true);
        zoom(true);
      }
      lastTap = undefined;
    } else {
      lastTap = { point: end, time: Date.now() };
    }
  }

  function onWheel(event: WheelEvent): void {
    if (isToolbar(event.target) || event.deltaY === 0) return;
    event.preventDefault();
    const unit = event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? window.innerHeight : 1;
    const delta = event.deltaY * unit;
    if (Math.sign(delta) !== Math.sign(wheelDelta)) wheelDelta = 0;
    wheelDelta += delta;
    if (Math.abs(wheelDelta) >= 100) {
      zoom(wheelDelta < 0);
      wheelDelta %= 100;
    }
  }

  container.addEventListener("touchstart", onTouchStart, { passive: false });
  container.addEventListener("touchmove", onTouchMove, { passive: false });
  container.addEventListener("touchend", onTouchEnd, { passive: false });
  container.addEventListener("touchcancel", cancel);
  container.addEventListener("wheel", onWheel, { passive: false });
  document.addEventListener("keydown", onKeydown, true);
  return () => {
    cancel();
    container.removeEventListener("touchstart", onTouchStart);
    container.removeEventListener("touchmove", onTouchMove);
    container.removeEventListener("touchend", onTouchEnd);
    container.removeEventListener("touchcancel", cancel);
    container.removeEventListener("wheel", onWheel);
    document.removeEventListener("keydown", onKeydown, true);
  };
}
