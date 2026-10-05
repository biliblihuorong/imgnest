import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NMessageProvider, NPagination, NSelect } from "naive-ui";
import { h } from "vue";
import { i18n } from "@vben/locales";
import { listAlbums } from "@/api/albums";
import { makeAlbum } from "@/components/albums/fixtures";
import {
  batchAlbums,
  batchDelete,
  batchPermission,
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
  batchDelete: vi.fn(),
  batchPermission: vi.fn(),
  batchAlbums: vi.fn(),
}));

vi.mock("@/api/albums", () => ({
  listAlbums: vi.fn(),
}));

const listImagesMock = vi.mocked(listImages);
const setImageVisibilityMock = vi.mocked(setImageVisibility);
const deleteImageMock = vi.mocked(deleteImage);
const getImageExifMock = vi.mocked(getImageExif);
const batchAlbumsMock = vi.mocked(batchAlbums);
const batchDeleteMock = vi.mocked(batchDelete);
const batchPermissionMock = vi.mocked(batchPermission);
const listAlbumsMock = vi.mocked(listAlbums);

enableAutoUnmount(afterEach);

function mountImages() {
  return mount(() => h(NMessageProvider, () => h(ImagesView)));
}

function findButton(wrapper: Awaited<ReturnType<typeof mountImages>>, text: string) {
  const button = [...wrapper.findAll("button")].find((node) => node.text().trim() === text);
  if (!button) {
    throw new Error(`找不到按钮：${text}`);
  }
  return button;
}

function bodyButton(text: string): HTMLButtonElement {
  const button = [...document.body.querySelectorAll("button")].find(
    (node) => node.textContent?.trim() === text,
  );
  if (!button) {
    throw new Error(`document.body 中找不到按钮：${text}`);
  }
  return button;
}

function batchItemOk(id: number) {
  return { id, status: 200, code: 0, message: "ok", data: null };
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
  it("页面提供唯一主标题，刷新当前列表时保留每页条数", async () => {
    const wrapper = mountImages();
    await flushPromises();
    expect(wrapper.findAll("h1")).toHaveLength(1);
    expect(wrapper.find("h1").text()).toBe("我的图片");

    wrapper.findComponent(NSelect).vm.$emit("update:value", 50);
    await flushPromises();
    const refresh = wrapper.findAll("button").find((button) => button.text() === "刷新");
    expect(refresh).toBeDefined();
    await refresh!.trigger("click");
    await flushPromises();

    expect(listImagesMock).toHaveBeenCalledTimes(3);
    expect(listImagesMock).toHaveBeenLastCalledWith({ page: 1, size: 50 });
    expect(wrapper.findAllComponents(ImageCard)).toHaveLength(2);
  });
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

  it("挂载时加载相册列表供筛选与移动使用", async () => {
    await mountImages();
    await flushPromises();

    expect(listAlbumsMock).toHaveBeenCalledWith({ page: 1, size: 100 });
  });

  it("相册筛选：未归类传 album_id=0，具体相册传 id，清空后不传", async () => {
    const wrapper = await mountImages();
    await flushPromises();

    const selects = wrapper.findAllComponents(NSelect);
    expect(selects.length).toBe(2);

    await selects[1]!.vm.$emit("update:value", 0);
    await flushPromises();
    expect(listImagesMock).toHaveBeenLastCalledWith({ page: 1, size: 20, album_id: 0 });

    await selects[1]!.vm.$emit("update:value", 7);
    await flushPromises();
    expect(listImagesMock).toHaveBeenLastCalledWith({ page: 1, size: 20, album_id: 7 });

    await selects[1]!.vm.$emit("update:value", null);
    await flushPromises();
    expect(listImagesMock).toHaveBeenLastCalledWith({ page: 1, size: 20 });
  });

  it("相册筛选切换后回到第 1 页并清空多选", async () => {
    const wrapper = await mountImages();
    await flushPromises();

    await wrapper.findAllComponents(ImageCard)[0].vm.$emit("select", true);
    await flushPromises();
    expect(wrapper.text()).toContain("已选 1 张");

    const selects = wrapper.findAllComponents(NSelect);
    await selects[1]!.vm.$emit("update:value", 7);
    await flushPromises();

    expect(listImagesMock).toHaveBeenLastCalledWith({ page: 1, size: 20, album_id: 7 });
    expect(wrapper.text()).not.toContain("已选 1 张");
  });

  it("多选两张后批量移动：调用 batchAlbums 并刷新、清空选择", async () => {
    batchAlbumsMock.mockResolvedValue([batchItemOk(1), batchItemOk(2)]);
    const wrapper = await mountImages();
    await flushPromises();

    const cards = wrapper.findAllComponents(ImageCard);
    await cards[0].vm.$emit("select", true);
    await cards[1].vm.$emit("select", true);
    await flushPromises();
    expect(wrapper.text()).toContain("已选 2 张");

    const selects = wrapper.findAllComponents(NSelect);
    expect(selects.length).toBe(3);
    await selects[2]!.vm.$emit("update:value", 7);
    await flushPromises();

    await findButton(wrapper, "移动到相册").trigger("click");
    await flushPromises();

    expect(batchAlbumsMock).toHaveBeenCalledWith([1, 2], 7);
    expect(listImagesMock).toHaveBeenCalledTimes(2);
    expect(wrapper.text()).not.toContain("已选 2 张");
    expect(document.body.textContent).toContain("移动成功 2 张");
  });

  it("未选择目标相册时批量移动给出警告且不发请求", async () => {
    const wrapper = await mountImages();
    await flushPromises();

    await wrapper.findAllComponents(ImageCard)[0].vm.$emit("select", true);
    await flushPromises();
    await findButton(wrapper, "移动到相册").trigger("click");
    await flushPromises();

    expect(batchAlbumsMock).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("请先选择目标相册");
  });

  it("批量移动部分失败时逐项反馈，成功的仍刷新列表", async () => {
    batchAlbumsMock.mockResolvedValue([
      { id: 1, status: 404, code: 30002, message: "图片不存在", data: null },
      batchItemOk(2),
    ]);
    const wrapper = await mountImages();
    await flushPromises();

    const cards = wrapper.findAllComponents(ImageCard);
    await cards[0].vm.$emit("select", true);
    await cards[1].vm.$emit("select", true);
    const selects = wrapper.findAllComponents(NSelect);
    await selects[2]!.vm.$emit("update:value", 8);
    await flushPromises();

    await findButton(wrapper, "移动到相册").trigger("click");
    await flushPromises();

    expect(document.body.textContent).toContain("移动成功 1 张");
    expect(document.body.textContent).toContain("移动失败（ID 1）：记录不存在或已被删除");
    expect(listImagesMock).toHaveBeenCalledTimes(2);
    expect(wrapper.text()).not.toContain("已选 2 张");
  });

  it("批量删除：Popconfirm 确认后调用 batchDelete 并清空选择", async () => {
    batchDeleteMock.mockResolvedValue([batchItemOk(1), batchItemOk(2)]);
    const wrapper = await mountImages();
    await flushPromises();

    const cards = wrapper.findAllComponents(ImageCard);
    await cards[0].vm.$emit("select", true);
    await cards[1].vm.$emit("select", true);
    await findButton(wrapper, "批量删除").trigger("click");
    await flushPromises();

    const panel = document.body.querySelector(".n-popconfirm__panel");
    expect(panel?.textContent).toContain("移入回收站");

    await bodyButton("确认删除").click();
    await flushPromises();

    expect(batchDeleteMock).toHaveBeenCalledWith([1, 2]);
    expect(document.body.textContent).toContain("删除成功 2 张");
    expect(wrapper.text()).not.toContain("已选 2 张");
  });

  it("批量改公开/私有：调用 batchPermission", async () => {
    batchPermissionMock.mockResolvedValue([batchItemOk(1)]);
    const wrapper = await mountImages();
    await flushPromises();

    await wrapper.findAllComponents(ImageCard)[0].vm.$emit("select", true);
    await findButton(wrapper, "设为公开").trigger("click");
    await flushPromises();
    expect(batchPermissionMock).toHaveBeenLastCalledWith([1], true);
    expect(document.body.textContent).toContain("设为公开成功 1 张");

    await wrapper.findAllComponents(ImageCard)[0].vm.$emit("select", true);
    await findButton(wrapper, "设为私有").trigger("click");
    await flushPromises();
    expect(batchPermissionMock).toHaveBeenLastCalledWith([1], false);
    expect(document.body.textContent).toContain("设为私有成功 1 张");
  });

  it("卡片「移动」下拉移动单张：batchAlbums([id], target)，不清空其他选择", async () => {
    batchAlbumsMock.mockResolvedValue([batchItemOk(1)]);
    const wrapper = await mountImages();
    await flushPromises();

    await wrapper.findAllComponents(ImageCard)[0].vm.$emit("move", 7);
    await flushPromises();

    expect(batchAlbumsMock).toHaveBeenCalledWith([1], 7);
    expect(document.body.textContent).toContain("移动成功 1 张");
  });

  it("批量条「取消」清空选择", async () => {
    const wrapper = await mountImages();
    await flushPromises();

    await wrapper.findAllComponents(ImageCard)[0].vm.$emit("select", true);
    await flushPromises();
    expect(wrapper.text()).toContain("已选 1 张");

    await findButton(wrapper, "取消").trigger("click");
    await flushPromises();

    expect(wrapper.text()).not.toContain("已选 1 张");
  });
});

describe("ImagesView migration regressions", () => {
  it("changes all visible controls to English without remounting", async () => {
    const wrapper = mountImages();
    await flushPromises();
    i18n.global.locale.value = "en-US";
    await flushPromises();
    expect(wrapper.find("h1").text()).toBe("My images");
    expect(wrapper.text()).toContain("42 images");
    expect(wrapper.text()).toContain("20 per page");
    expect(wrapper.text()).not.toContain("我的图片");
  });
  it("ignores an old page result after newer navigation", async () => {
    let resolveOld!: (value: Awaited<ReturnType<typeof listImages>>) => void;
    listImagesMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveOld = resolve;
        }),
    );
    const wrapper = mountImages();
    wrapper.findComponent(NPagination).vm.$emit("update:page", 2);
    await flushPromises();
    resolveOld({ items: [makeImage({ id: 99, name: "stale.png" })], total: 1, page: 1, size: 20 });
    await flushPromises();
    expect(wrapper.text()).not.toContain("stale.png");
    expect(wrapper.text()).toContain("a.png");
  });
  it("does not duplicate a pending visibility mutation", async () => {
    setImageVisibilityMock.mockReturnValue(new Promise(() => {}));
    const wrapper = mountImages();
    await flushPromises();
    const card = wrapper.findComponent(ImageCard);
    card.vm.$emit("toggle", true);
    card.vm.$emit("toggle", true);
    await flushPromises();
    expect(setImageVisibilityMock).toHaveBeenCalledTimes(1);
  });
  it("deduplicates page-local selections", async () => {
    const wrapper = mountImages();
    await flushPromises();
    const card = wrapper.findComponent(ImageCard);
    card.vm.$emit("select", true);
    card.vm.$emit("select", true);
    await flushPromises();
    expect(wrapper.text()).toContain("已选 1 张");
  });
});
it("does not keep previous-page images actionable when the new page fails and offers retry", async () => {
  const wrapper = mountImages();
  await flushPromises();
  listImagesMock.mockRejectedValueOnce(new Error("offline"));
  wrapper.findComponent(NPagination).vm.$emit("update:page", 2);
  await flushPromises();
  expect(wrapper.findAllComponents(ImageCard)).toHaveLength(0);
  expect(wrapper.find(".n-alert").text()).toContain("图片列表加载失败");
  const retry = wrapper.findAll("button").find((button) => button.text() === "重试");
  expect(retry).toBeDefined();
  await retry!.trigger("click");
  await flushPromises();
  expect(wrapper.findAllComponents(ImageCard)).toHaveLength(2);
});
it("keeps an already visible error message in sync with the selected language", async () => {
  setImageVisibilityMock.mockRejectedValue(new Error("network"));
  const wrapper = mountImages();
  await flushPromises();
  wrapper.findComponent(ImageCard).vm.$emit("toggle", true);
  await flushPromises();
  i18n.global.locale.value = "en-US";
  await flushPromises();
  expect(document.body.textContent).toContain("Could not change visibility");
  expect(document.body.textContent).not.toContain("可见性修改失败");
});
