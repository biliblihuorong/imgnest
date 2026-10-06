import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NDialogProvider, NMessageProvider } from "naive-ui";
import { h } from "vue";
import type { ImageView } from "@/api/images";
import { makeImage } from "@/components/images/fixtures";
import ImageCopyDrawer from "./ImageCopyDrawer.vue";

enableAutoUnmount(afterEach);

function mountDrawer(image: ImageView | null, show = true) {
  return mount(
    () => h(NMessageProvider, () => h(NDialogProvider, () => h(ImageCopyDrawer, { show, image }))),
    { attachTo: document.body },
  );
}

beforeEach(() => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe(): void {}
      unobserve(): void {}
      disconnect(): void {}
    },
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
  document.body.innerHTML = "";
});

describe("ImageCopyDrawer", () => {
  it("三段版本各显示链接文本与四个格式复制按钮", () => {
    const wrapper = mountDrawer(makeImage());
    const body = document.body.textContent ?? "";

    expect(body).toContain("复制链接");
    expect(body).toContain("原图");
    expect(body).toContain("WebP");
    expect(body).toContain("缩略图");
    expect(body).toContain(makeImage().links.original);
    for (const label of ["复制 URL", "复制 Markdown", "复制 HTML", "复制 BBCode"]) {
      expect(body).toContain(label);
    }
    wrapper.unmount();
  });

  it("点击「WebP 复制 Markdown」写入对应的拼接文本", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal("navigator", { userAgent: "vitest", clipboard: { writeText } });
    const image = makeImage();
    const wrapper = mountDrawer(image);

    const section = [...document.body.querySelectorAll(".copy-drawer__section")].find((node) =>
      node.textContent?.includes("WebP"),
    );
    if (!section) throw new Error("webp section not found");
    const button = [...section.querySelectorAll("button")].find(
      (node) => node.textContent?.trim() === "复制 Markdown",
    );
    if (!button) throw new Error("markdown button not found");
    button.click();
    await flushPromises();

    expect(writeText).toHaveBeenCalledWith(`![photo.png](${image.links.webp})`);
    wrapper.unmount();
    vi.unstubAllGlobals();
  });

  it("缺失版本禁用复制并显示未生成标记", () => {
    const image = makeImage();
    const wrapper = mountDrawer({ ...image, links: { ...image.links, webp: "" } });
    const section = [...document.body.querySelectorAll(".copy-drawer__section")].find((node) =>
      node.textContent?.includes("WebP"),
    );
    expect(section?.textContent).toContain("未生成");
    const buttons = [...(section?.querySelectorAll("button") ?? [])];
    expect(buttons.length).toBeGreaterThan(0);
    expect(buttons.every((node) => (node as HTMLButtonElement).disabled)).toBe(true);
    wrapper.unmount();
  });

  it("show=false 不渲染抽屉内容", () => {
    const wrapper = mountDrawer(makeImage(), false);
    expect(document.body.textContent ?? "").not.toContain("复制链接");
    wrapper.unmount();
  });
});
