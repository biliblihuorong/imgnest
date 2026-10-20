import { i18n } from "@vben/locales";
import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { previewGallery } from "@/api/admin";
import { makeImage } from "@/components/images/fixtures";
import GalleryPreviewPanel from "./GalleryPreviewPanel.vue";

vi.mock("@/api/admin", () => ({ previewGallery: vi.fn() }));
enableAutoUnmount(afterEach);
const previewMock = vi.mocked(previewGallery);

beforeEach(() => {
  i18n.global.locale.value = "zh-CN";
  vi.resetAllMocks();
  previewMock.mockResolvedValue({ items: [], total: 7, page: 1, size: 12 });
});

it("按未保存的相册范围更新展示数量，并丢弃旧请求", async () => {
  let finish!: (value: Awaited<ReturnType<typeof previewGallery>>) => void;
  previewMock.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const wrapper = mount(GalleryPreviewPanel, { props: { publicAlbumsOnly: false } });
  await wrapper.setProps({ publicAlbumsOnly: true });
  await flushPromises();
  expect(previewMock).toHaveBeenNthCalledWith(2, true);
  expect(wrapper.text()).toContain("7 张");
  finish({ items: [], total: 99, page: 1, size: 12 });
  await flushPromises();
  expect(wrapper.text()).not.toContain("99 张");
  expect(wrapper.text()).toContain("7 张");
});

it("预览使用公开画廊卡片，不保存设置；失败后可重试", async () => {
  previewMock.mockRejectedValueOnce(new Error("offline"));
  const wrapper = mount(GalleryPreviewPanel, { props: { publicAlbumsOnly: false } });
  await flushPromises();
  expect(wrapper.text()).toContain("预览加载失败");
  expect(wrapper.find("[data-gallery-count]").exists()).toBe(false);
  previewMock.mockResolvedValue({
    items: [{ ...makeImage({ name: "public.png" }), uploader: "Alice" }],
    total: 1,
    page: 1,
    size: 12,
  });
  await wrapper.find("[data-action='retry-preview']").trigger("click");
  await flushPromises();
  await wrapper.find("[data-action='toggle-preview']").trigger("click");
  expect(wrapper.find(".gallery-card").text()).toContain("public.png");
  expect(wrapper.text()).toContain("Alice");
  expect(wrapper.find("form").exists()).toBe(false);
});
