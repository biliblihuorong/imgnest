import { onBeforeUnmount, onMounted } from "vue";

/**
 * 灯箱触屏左右滑动切换上/下一张：MutationObserver 监听 Naive UI 预览遮罩出现后绑定
 * touch 手势，水平滑动超过阈值时向 document 派发键盘左右箭头——Naive UI 的预览组件
 * 已全局监听 keydown，NImageGroup 场景下自动翻页；单图预览时事件无人消费，无害。
 */
export function usePreviewSwipe(): void {
  let observer: MutationObserver | undefined;
  let cleanup: (() => void) | undefined;
  const swipeThreshold = 50;

  function attachSwipe(container: HTMLElement): void {
    let startX = 0;
    let startY = 0;
    let tracking = false;

    function onTouchStart(e: TouchEvent): void {
      if (e.touches.length !== 1) {
        tracking = false;
        return;
      }
      startX = e.touches[0]!.clientX;
      startY = e.touches[0]!.clientY;
      tracking = true;
    }

    function onTouchEnd(e: TouchEvent): void {
      if (!tracking || e.changedTouches.length !== 1) {
        tracking = false;
        return;
      }
      const dx = e.changedTouches[0]!.clientX - startX;
      const dy = e.changedTouches[0]!.clientY - startY;
      tracking = false;
      if (Math.abs(dx) < swipeThreshold || Math.abs(dy) > Math.abs(dx) * 0.7) return;

      document.dispatchEvent(
        new KeyboardEvent("keydown", {
          key: dx < 0 ? "ArrowRight" : "ArrowLeft",
          bubbles: true,
        }),
      );
    }

    container.addEventListener("touchstart", onTouchStart, { passive: true });
    container.addEventListener("touchend", onTouchEnd, { passive: true });

    cleanup = () => {
      container.removeEventListener("touchstart", onTouchStart);
      container.removeEventListener("touchend", onTouchEnd);
    };
  }

  function findPreviewContainer(root: Node): HTMLElement | null {
    if (!(root instanceof HTMLElement)) return null;
    if (root.classList?.contains("n-image-preview-container")) return root;
    return root.querySelector(".n-image-preview-container");
  }

  function startObserving(): void {
    const existing = document.querySelector(".n-image-preview-container");
    if (existing) attachSwipe(existing as HTMLElement);

    observer = new MutationObserver((mutations) => {
      for (const mutation of mutations) {
        for (const node of mutation.addedNodes) {
          const container = findPreviewContainer(node);
          if (container) {
            cleanup?.();
            attachSwipe(container);
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
  });
}
