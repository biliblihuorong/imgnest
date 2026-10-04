import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NMessageProvider, NPagination, NSelect } from "naive-ui";
import { h } from "vue";
import {
  deleteImage,
  getImageExif,
  listImages,
  setImageVisibility,
} from "@/api/images";
import ImageCard from "@/components/images/ImageCard.vue";
import { makeExif, makeImage } from "@/components/images/fixtures";
import ImagesView from "./ImagesView.vue";

vi.mock("@/api/images", () => ({
  listImages: vi.fn(),
  setImageVisibility: vi.fn(),
  deleteImage: vi.fn(),
  getImageExif: vi.fn(),
  listTrash: vi.fn(),
  restoreImages: vi.fn(),
  purgeImages: vi.fn(),
}));

const listImagesMock = vi.mocked(listImages);
const setImageVisibilityMock = vi.mocked(setImageVisibility);
const deleteImageMock = vi.mocked(deleteImage);
const getImageExifMock = vi.mocked(getImageExif);

enableAutoUnmount(afterEach);

function mountImages() {
  return mount(() => h(NMessageProvider, () => h(ImagesView)));
}

beforeEach(() => {
  vi.resetAllMocks();
  listImagesMock.mockResolvedValue({
    items: [
      makeImage({ id: 1, name: "a.png", size: 2048 }),
      makeImage({ id: 2, name: "b.jpg", is_public: true }),
    ],
    total: 42,
    page: 1,
    size: 20,
  });
});

describe("ImagesView", () => {
  it("挂载后按 page=1&size=20 加载并渲染网格与总数", async () => {
    const wrapper = await mountImages();
    await flushPromises();

    expect(listImagesMock).toHaveBeenCalledWith({ page: 1, size: 20 });
    expect(wrapper.text()).toContain("共 42 张图片");
    expect(wrapper.text()).toContain("a.png");
    expect(wrapper.text()).toContain("2.0 KB");
    expect(wrapper.findAllComponents(ImageCard)).toHaveLength(2);
  });

  it("列表为空时显示空状态", async () => {
    listImagesMock.mockResolvedValue({ items: [], total: 0, page: 1, size: 20 });
    const wrapper = await mountImages();
    await flushPromises();

    expect(wrapper.text()).toContain("还没有图片");
  });

  it("翻页时携带新的 page 参数", async () => {
    const wrapper = await mountImages();
    await flushPromises();

    wrapper.findComponent(NPagination).vm.$emit("update:page", 2);
    await flushPromises();

    expect(listImagesMock).toHaveBeenLastCalledWith({ page: 2, size: 20 });
  });

  it("修改每页条数时从第 1 页重新加载", async () => {
    const wrapper = await mountImages();
    await flushPromises();

    wrapper.findComponent(NSelect).vm.$emit("update:value", 50);
    await flushPromises();

    expect(listImagesMock).toHaveBeenLastCalledWith({ page: 1, size: 50 });
  });

  it("可见性切换成功：调用 PATCH 并更新列表项", async () => {
    setImageVisibilityMock.mockResolvedValue(makeImage({ id: 1, is_public: true }));
    const wrapper = await mountImages();
    await flushPromises();

    wrapper.findAllComponents(ImageCard)[0].vm.$emit("toggle", true);
    await flushPromises();

    expect(setImageVisibilityMock).toHaveBeenCalledWith(1, true);
    expect(wrapper.findAllComponents(ImageCard)[0].props("image").is_public).toBe(true);
    expect(document.body.textContent).toContain("已设为公开");
  });

  it("可见性切换失败：列表项不变（开关回滚）并提示错误", async () => {
    setImageVisibilityMock.mockRejectedValue(new Error("网络错误"));
    const wrapper = await mountImages();
    await flushPromises();

    wrapper.findAllComponents(ImageCard)[0].vm.$emit("toggle", true);
    await flushPromises();

    expect(setImageVisibilityMock).toHaveBeenCalledWith(1, true);
    expect(wrapper.findAllComponents(ImageCard)[0].props("image").is_public).toBe(false);
    expect(document.body.textContent).toContain("可见性修改失败");
  });

  it("删除确认后调用 DELETE 并刷新列表", async () => {
    deleteImageMock.mockResolvedValue(null);
    const wrapper = await mountImages();
    await flushPromises();
    expect(listImagesMock).toHaveBeenCalledTimes(1);

    wrapper.findAllComponents(ImageCard)[0].vm.$emit("remove");
    await flushPromises();

    expect(deleteImageMock).toHaveBeenCalledWith(1);
    expect(listImagesMock).toHaveBeenCalledTimes(2);
    expect(document.body.textContent).toContain("已移入回收站");
  });

  it("点击卡片打开详情抽屉并懒加载 EXIF", async () => {
    getImageExifMock.mockResolvedValue(makeExif({ image_id: 1 }));
    const wrapper = await mountImages();
    await flushPromises();
    expect(getImageExifMock).not.toHaveBeenCalled();

    wrapper.findAllComponents(ImageCard)[0].vm.$emit("open");
    await flushPromises();

    expect(getImageExifMock).toHaveBeenCalledWith(1);
    expect(document.body.textContent).toContain("仅本人可见");
  });
});
