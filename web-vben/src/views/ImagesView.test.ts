import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NDialogProvider, NMessageProvider } from "naive-ui";
import { h } from "vue";
import { i18n } from "@vben/locales";
import { listAlbums } from "@/api/albums";
import { listImages } from "@/api/images";
import { makeAlbum } from "@/components/albums/fixtures";
import { makeImage } from "@/components/images/fixtures";
import ImageLibrary from "@/components/images/ImageLibrary.vue";
import ImagesView from "./ImagesView.vue";

vi.mock("@/api/images", () => ({
  listImages: vi.fn(),
  setImageVisibility: vi.fn(),
  deleteImage: vi.fn(),
  getImageExif: vi.fn(),
  listTrash: vi.fn(),
  restoreImages: vi.fn(),
  purgeImages: vi.fn(),
  batchDelete: vi.fn(),
  batchPermission: vi.fn(),
  batchAlbums: vi.fn(),
}));

vi.mock("@/api/albums", () => ({
  listAlbums: vi.fn(),
}));

const listImagesMock = vi.mocked(listImages);
const listAlbumsMock = vi.mocked(listAlbums);

enableAutoUnmount(afterEach);

function mountView() {
  return mount(() => h(NMessageProvider, () => h(NDialogProvider, () => h(ImagesView))));
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
  listAlbumsMock.mockResolvedValue({
    items: [makeAlbum({ id: 7, name: "旅行" }), makeAlbum({ id: 8, name: "工作" })],
    total: 2,
    page: 1,
    size: 100,
  });
});

describe("ImagesView", () => {
  it("页面提供唯一主标题并渲染图片库与回收站两个标签页", async () => {
    const wrapper = mountView();
    await flushPromises();

    expect(wrapper.findAll("h1")).toHaveLength(1);
    expect(wrapper.find("h1").text()).toBe("我的图片");
    expect(wrapper.findAllComponents(ImageLibrary)).toHaveLength(1);
    expect(wrapper.text()).toContain("回收站");
  });

  it("changes the page heading to English without remounting", async () => {
    const wrapper = mountView();
    await flushPromises();
    i18n.global.locale.value = "en-US";
    await flushPromises();
    expect(wrapper.find("h1").text()).toBe("My images");
    expect(wrapper.text()).not.toContain("我的图片");
  });
});
