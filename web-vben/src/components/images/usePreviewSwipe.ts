import { onBeforeUnmount, onMounted } from "vue";

/**
 * 灯箱触屏/鼠标增强：在 Naive UI 预览遮罩上绑定
 * - 水平滑动翻页（>50px 位移 + 速度阈值 + 排除倾角 >35° 的斜滑）
 * - 单指拖动平移（放大后）
 * - 双指捏合缩放
 * - 双击/双触缩放
 * - 滚轮缩放（PC）
 * MutationObserver 监听预览容器出现后绑定；工具栏区域不触发翻页。
 */
export function usePreviewSwipe(): void {
  // 全局去重：同页多处调用只挂一套监听
  if ((window as any).__imgnestPreviewSwipeAttached) return;
  (window as any).__imgnestPreviewSwipeAttached = true;

  let observer: MutationObserver | undefined;
  let cleanup: (() => void) | undefined;

  const swipeThreshold = 50;
  const swipeVelocityThreshold = 0.3; // px/ms，约 300px/s
  const pinchZoomSensitivity = 0.005;
  const wheelZoomSensitivity = 0.001;
  const maxScale = 5;
  const minScale = 0.5;

  function attachEnhancements(container: HTMLElement): void {
    // 工具栏选择器：Naive UI 预览的工具按钮区域
    const toolbarSelector = ".n-image-preview-toolbar, [class*='toolbar']";

    let startX = 0;
    let startY = 0;
    let startTime = 0;
    let tracking = false;
    let pinchStartDistance = 0;
    let pinchStartScale = 1;
    let currentScale = 1;
    let offsetX = 0;
    let offsetY = 0;
    let isPanning = false;
    let panStartX = 0;
    let panStartY = 0;
    let lastTapTime = 0;

    function getTouchDistance(t1: Touch, t2: Touch): number {
      const dx = t2.clientX - t1.clientX;
      const dy = t2.clientY - t1.clientY;
      return Math.sqrt(dx * dx + dy * dy);
    }

    function isToolbar(target: EventTarget): boolean {
      const el = target as HTMLElement;
      return !!el.closest(toolbarSelector);
    }

    function onTouchStart(e: TouchEvent): void {
      if (isToolbar(e.target)) return;

      if (e.touches.length === 2) {
        // 双指捏合开始
        tracking = false;
        pinchStartDistance = getTouchDistance(e.touches[0]!, e.touches[1]!);
        pinchStartScale = currentScale;
        return;
      }

      if (e.touches.length !== 1) {
        tracking = false;
        return;
      }

      // 双击检测
      const now = Date.now();
      if (now - lastTapTime < 300) {
        // 双击缩放
        currentScale = currentScale > 1 ? 1 : 2;
        applyTransform();
        lastTapTime = 0;
        tracking = false;
        return;
      }
      lastTapTime = now;

      startX = e.touches[0]!.clientX;
      startY = e.touches[0]!.clientY;
      startTime = now;
      tracking = true;

      // 如果已放大，进入平移模式
      if (currentScale > 1) {
        isPanning = true;
        panStartX = offsetX;
        panStartY = offsetY;
      }
    }

    function onTouchMove(e: TouchEvent): void {
      if (isToolbar(e.target)) return;

      if (e.touches.length === 2 && pinchStartDistance > 0) {
        // 双指捏合缩放
        e.preventDefault();
        const distance = getTouchDistance(e.touches[0]!, e.touches[1]!);
        const scaleDelta = (distance - pinchStartDistance) * pinchZoomSensitivity;
        currentScale = Math.min(maxScale, Math.max(minScale, pinchStartScale + scaleDelta));
        applyTransform();
        return;
      }

      if (!tracking || e.touches.length !== 1) return;

      if (isPanning && currentScale > 1) {
        // 单指平移
        e.preventDefault();
        const dx = e.touches[0]!.clientX - startX;
        const dy = e.touches[0]!.clientY - startY;
        offsetX = panStartX + dx;
        offsetY = panStartY + dy;
        applyTransform();
        return;
      }

      // 否则继续跟踪滑动（用于翻页）
      const dx = e.touches[0]!.clientX - startX;
      const dy = e.touches[0]!.clientY - startY;
      // 如果垂直位移过大，取消翻页跟踪
      if (Math.abs(dy) > Math.abs(dx) * 0.7) {
        tracking = false;
      }
    }

    function onTouchEnd(e: TouchEvent): void {
      if (isToolbar(e.target)) return;

      if (e.touches.length === 0) {
        pinchStartDistance = 0;
      }

      if (!tracking || e.changedTouches.length !== 1) {
        tracking = false;
        isPanning = false;
        return;
      }

      const endX = e.changedTouches[0]!.clientX;
      const endY = e.changedTouches[0]!.clientY;
      const dx = endX - startX;
      const dy = endY - startY;
      const dt = Date.now() - startTime;
      const velocity = Math.abs(dx) / dt;

      tracking = false;
      isPanning = false;

      // 翻页判断：位移阈值 + 速度阈值 + 水平主导
      if (
        Math.abs(dx) >= swipeThreshold &&
        velocity >= swipeVelocityThreshold &&
        Math.abs(dy) <= Math.abs(dx) * 0.7
      ) {
        document.dispatchEvent(
          new KeyboardEvent("keydown", {
            key: dx < 0 ? "ArrowRight" : "ArrowLeft",
            bubbles: true,
          }),
        );
      }
    }

    function onWheel(e: WheelEvent): void {
      if (isToolbar(e.target)) return;

      e.preventDefault();
      const delta = -e.deltaY * wheelZoomSensitivity;
      currentScale = Math.min(maxScale, Math.max(minScale, currentScale + delta));
      applyTransform();
    }

    function applyTransform(): void {
      const img = container.querySelector("img");
      if (img) {
        img.style.transform = `translate(${offsetX}px, ${offsetY}px) scale(${currentScale})`;
        img.style.transformOrigin = "center center";
        img.style.transition = "transform 0.15s ease-out";
      }
    }

    function resetTransform(): void {
      currentScale = 1;
      offsetX = 0;
      offsetY = 0;
      const img = container.querySelector("img");
      if (img) {
        img.style.transform = "";
        img.style.transformOrigin = "";
      }
    }

    // 监听 Naive UI 的翻页事件，重置变换
    const resetOnNavigate = () => resetTransform();
    document.addEventListener("keydown", resetOnNavigate);

    container.addEventListener("touchstart", onTouchStart, { passive: false });
    container.addEventListener("touchmove", onTouchMove, { passive: false });
    container.addEventListener("touchend", onTouchEnd, { passive: true });
    container.addEventListener("wheel", onWheel, { passive: false });

    cleanup = () => {
      container.removeEventListener("touchstart", onTouchStart);
      container.removeEventListener("touchmove", onTouchMove);
      container.removeEventListener("touchend", onTouchEnd);
      container.removeEventListener("wheel", onWheel);
      document.removeEventListener("keydown", resetOnNavigate);
    };
  }

  function findPreviewContainer(root: Node): HTMLElement | null {
    if (!(root instanceof HTMLElement)) return null;
    if (root.classList?.contains("n-image-preview-container")) return root;
    return root.querySelector(".n-image-preview-container");
  }

  function startObserving(): void {
    const existing = document.querySelector(".n-image-preview-container");
    if (existing) attachEnhancements(existing as HTMLElement);

    observer = new MutationObserver((mutations) => {
      for (const mutation of mutations) {
        for (const node of mutation.addedNodes) {
          const container = findPreviewContainer(node);
          if (container) {
            cleanup?.();
            attachEnhancements(container);
          }
        }
      }
    });
    observer.observe(document.body, { childList: true, subtree: true });
  }

  onMounted(startObserving);
  onBeforeUnmount(() => {
    cleanup?.();
    observer?.disconnect();
    (window as any).__imgnestPreviewSwipeAttached = false;
  });
}
