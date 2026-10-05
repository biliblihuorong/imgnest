import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  NDatePicker,
  NDialogProvider,
  NDropdown,
  NInput,
  NInputNumber,
  NMessageProvider,
  NPagination,
  NSelect,
} from "naive-ui";
import { h } from "vue";
import { i18n } from "@vben/locales";
import { listAlbums } from "@/api/albums";
import {
  batchAlbums,
  batchDelete,
  batchPermission,
  deleteImage,
  getImageExif,
  listImages,
  setImageVisibility,
  type ListParams,
} from "@/api/images";
import { makeAlbum } from "@/components/albums/fixtures";
import { fetchProtectedThumbnail } from "@/api/thumbnails";
import ImageCard from "@/components/images/ImageCard.vue";
import ImageDetailDrawer from "@/components/images/ImageDetailDrawer.vue";
import { makeExif, makeImage } from "@/components/images/fixtures";
import { ALBUM_FILTER_ALL } from "@/components/images/useImageLibrary";
import ImageLibrary from "./ImageLibrary.vue";

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

vi.mock("@/api/thumbnails", () => ({
  fetchProtectedThumbnail: vi.fn(),
}));

const listImagesMock = vi.mocked(listImages);
const setImageVisibilityMock = vi.mocked(setImageVisibility);
const deleteImageMock = vi.mocked(deleteImage);
const getImageExifMock = vi.mocked(getImageExif);
const batchAlbumsMock = vi.mocked(batchAlbums);
const batchDeleteMock = vi.mocked(batchDelete);
const batchPermissionMock = vi.mocked(batchPermission);
const listAlbumsMock = vi.mocked(listAlbums);

const fetchProtectedThumbnailMock = vi.mocked(fetchProtectedThumbnail);

enableAutoUnmount(afterEach);

/** load() 总是携带完整参数对象（缺省过滤键为 undefined）。 */
function params(overrides: Partial<ListParams> = {}): ListParams {
  return {
    album_id: undefined,
    q: undefined,
    order: undefined,
    min_size: undefined,
    max_size: undefined,
    from: undefined,
    to: undefined,
    ...overrides,
  };
}

function mountLibrary(props: Record<string, unknown> = {}) {
  return mount(
    () =>
      h(NMessageProvider, () => h(NDialogProvider, () => h(ImageLibrary, props))),
    { attachTo: document.body },
  );
}

function findButton(wrapper: Awaited<ReturnType<typeof mountLibrary>>, text: string) {
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
  URL.createObjectURL = vi.fn().mockReturnValue("blob:library-thumb");
  URL.revokeObjectURL = vi.fn();
  fetchProtectedThumbnailMock.mockResolvedValue(
    new Blob(["image"], { type: "image/webp" }),
  );
});

describe("ImageLibrary", () => {
  it("挂载后按 page=1&size=20 加载并渲染网格与总数", async () => {
    const wrapper = await mountLibrary();
    await flushPromises();

    expect(listImagesMock).toHaveBeenCalledWith(params({ page: 1, size: 20 }));
    expect(wrapper.text()).toContain("共 42 张图片");
    expect(wrapper.text()).toContain("a.png");
    expect(wrapper.text()).toContain("2.0 KB");
    expect(wrapper.findAllComponents(ImageCard)).toHaveLength(2);
  });

  it("列表为空时显示空状态", async () => {
    listImagesMock.mockResolvedValue({ items: [], total: 0, page: 1, size: 20 });
    const wrapper = await mountLibrary();
    await flushPromises();

    expect(wrapper.text()).toContain("还没有图片");
  });

  it("翻页时携带新的 page 参数", async () => {
    const wrapper = await mountLibrary();
    await flushPromises();

    wrapper.findComponent(NPagination).vm.$emit("update:page", 2);
    await flushPromises();

    expect(listImagesMock).toHaveBeenLastCalledWith(params({ page: 2, size: 20 }));
  });

  it("修改每页条数时从第 1 页重新加载", async () => {
    const wrapper = await mountLibrary();
    await flushPromises();

    // NSelect 顺序：相册筛选(0)、排序(1)、每页条数(2)
    const selects = wrapper.findAllComponents(NSelect);
    expect(selects.length).toBe(3);
    await selects[2]!.vm.$emit("update:value", 50);
    await flushPromises();

    expect(listImagesMock).toHaveBeenLastCalledWith(params({ page: 1, size: 50 }));
  });

  it("相册筛选：显式「全部」不传 album_id，未归类传 0，指定相册传 id", async () => {
    const wrapper = await mountLibrary();
    await flushPromises();

    const selects = wrapper.findAllComponents(NSelect);
    await selects[0]!.vm.$emit("update:value", 0);
    await flushPromises();
    expect(listImagesMock).toHaveBeenLastCalledWith(params({ page: 1, size: 20, album_id: 0 }));

    await selects[0]!.vm.$emit("update:value", 7);
    await flushPromises();
    expect(listImagesMock).toHaveBeenLastCalledWith(params({ page: 1, size: 20, album_id: 7 }));

    await selects[0]!.vm.$emit("update:value", ALBUM_FILTER_ALL);
    await flushPromises();
    expect(listImagesMock).toHaveBeenLastCalledWith(params({ page: 1, size: 20 }));
  });

  it("排序切换携带 order 参数并回到第 1 页", async () => {
    const wrapper = await mountLibrary();
    await flushPromises();

    const selects = wrapper.findAllComponents(NSelect);
    await selects[1]!.vm.$emit("update:value", "largest");
    await flushPromises();

    expect(listImagesMock).toHaveBeenLastCalledWith(
      params({ page: 1, size: 20, order: "largest" }),
    );
  });

  it("大小范围过滤换算为字节", async () => {
    const wrapper = await mountLibrary();
    await flushPromises();

    const numbers = wrapper.findAllComponents(NInputNumber);
    await numbers[0]!.vm.$emit("update:value", 1);
    await numbers[1]!.vm.$emit("update:value", 5);
    await flushPromises();

    expect(listImagesMock).toHaveBeenLastCalledWith(
      params({ page: 1, size: 20, min_size: 1024 * 1024, max_size: 5 * 1024 * 1024 }),
    );
  });

  it("时间范围过滤序列化为 ISO 字符串", async () => {
    const wrapper = await mountLibrary();
    await flushPromises();

    const from = Date.UTC(2026, 9, 1, 0, 0, 0);
    const to = Date.UTC(2026, 9, 4, 23, 59, 59);
    wrapper.findComponent(NDatePicker)
      .vm.$emit("update:value", [from, to] as [number, number]);
    await flushPromises();

    expect(listImagesMock).toHaveBeenLastCalledWith(
      params({ page: 1, size: 20, from: new Date(from).toISOString(), to: new Date(to).toISOString() }),
    );
  });

  it("统一搜索防抖后携带 q 重查", async () => {
    const wrapper = await mountLibrary();
    await flushPromises();
    expect(listImagesMock).toHaveBeenCalledTimes(1);

    const inputs = wrapper.findAllComponents(NInput);
    await inputs[0]!.vm.$emit("update:value", "vacation");
    await new Promise((resolve) => setTimeout(resolve, 450));
    await flushPromises();

    expect(listImagesMock).toHaveBeenLastCalledWith(params({ page: 1, size: 20, q: "vacation" }));
  });

  it("有筛选时显示清除按钮，点击恢复默认参数", async () => {
    const wrapper = await mountLibrary();
    await flushPromises();

    const selects = wrapper.findAllComponents(NSelect);
    await selects[1]!.vm.$emit("update:value", "smallest");
    await flushPromises();
    expect(listImagesMock).toHaveBeenLastCalledWith(
      params({ page: 1, size: 20, order: "smallest" }),
    );

    await findButton(wrapper, "清除筛选").trigger("click");
    await flushPromises();

    expect(listImagesMock).toHaveBeenLastCalledWith(params({ page: 1, size: 20 }));
  });

  it("锁定相册模式：固定 album_id 且隐藏相册筛选", async () => {
    const wrapper = await mountLibrary({ lockedAlbumId: 7 });
    await flushPromises();

    expect(listImagesMock).toHaveBeenCalledWith(params({ page: 1, size: 20, album_id: 7 }));
    const selects = wrapper.findAllComponents(NSelect);
    expect(selects.length).toBe(2);
  });

  it("可见性切换成功：调用 PATCH 并更新列表项", async () => {
    setImageVisibilityMock.mockResolvedValue(makeImage({ id: 1, is_public: true }));
    const wrapper = await mountLibrary();
    await flushPromises();

    wrapper.findAllComponents(ImageCard)[0].vm.$emit("toggle", true);
    await flushPromises();

    expect(setImageVisibilityMock).toHaveBeenCalledWith(1, true);
    expect(wrapper.findAllComponents(ImageCard)[0].props("image").is_public).toBe(true);
    expect(document.body.textContent).toContain("已设为公开");
  });

  it("可见性切换失败：列表项不变（开关回滚）并提示错误", async () => {
    setImageVisibilityMock.mockRejectedValue(new Error("网络错误"));
    const wrapper = await mountLibrary();
    await flushPromises();

    wrapper.findAllComponents(ImageCard)[0].vm.$emit("toggle", true);
    await flushPromises();

    expect(setImageVisibilityMock).toHaveBeenCalledWith(1, true);
    expect(wrapper.findAllComponents(ImageCard)[0].props("image").is_public).toBe(false);
    expect(document.body.textContent).toContain("可见性修改失败");
  });

  it("删除确认后调用 DELETE 并刷新列表", async () => {
    deleteImageMock.mockResolvedValue(null);
    const wrapper = await mountLibrary();
    await flushPromises();
    expect(listImagesMock).toHaveBeenCalledTimes(1);

    wrapper.findAllComponents(ImageCard)[0].vm.$emit("remove");
    await flushPromises();

    expect(deleteImageMock).toHaveBeenCalledWith(1);
    expect(listImagesMock).toHaveBeenCalledTimes(2);
    expect(document.body.textContent).toContain("已移入回收站");
  });

  it("点击文件名打开详情抽屉并懒加载 EXIF", async () => {
    getImageExifMock.mockResolvedValue(makeExif({ image_id: 1 }));
    const wrapper = await mountLibrary();
    await flushPromises();
    expect(getImageExifMock).not.toHaveBeenCalled();

    wrapper.findAllComponents(ImageCard)[0].vm.$emit("open");
    await flushPromises();

    expect(getImageExifMock).toHaveBeenCalledWith(1);
    expect(document.body.textContent).toContain("仅本人可见");
  });

  it("点击缩略图打开分组灯箱（WebP 优先，支持翻页）", async () => {
    const wrapper = await mountLibrary();
    await flushPromises();

    const thumb = wrapper.find(".image-card__thumb img");
    expect(thumb.exists()).toBe(true);
    await thumb.trigger("click");
    await flushPromises();

    expect(document.querySelector(".n-image-preview")).not.toBeNull();
  });

  it("右键菜单：复制 URL / 打开属性 / 切换公开私有", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal("navigator", { userAgent: "vitest", clipboard: { writeText } });
    getImageExifMock.mockResolvedValue(makeExif({ image_id: 1 }));
    setImageVisibilityMock.mockResolvedValue(makeImage({ id: 1, is_public: true }));
    const wrapper = await mountLibrary();
    await flushPromises();

    const card = wrapper.findAllComponents(ImageCard)[0];
    card.vm.$emit("menu", new MouseEvent("contextmenu"));
    await flushPromises();

    // 菜单渲染到 body
    expect(document.body.textContent).toContain("属性");

    const dropdown = wrapper
      .findAllComponents(NDropdown)
      .find((node) => node.props("trigger") === "manual")!;
    // 二级复制：WebP 的 URL
    await dropdown.vm.$emit("select", "copy-webp:url");
    await flushPromises();
    expect(writeText).toHaveBeenCalledWith(makeImage().links.webp);
    // 二级复制：原图 Markdown
    await dropdown.vm.$emit("select", "copy-original:markdown");
    await flushPromises();
    expect(writeText).toHaveBeenLastCalledWith(`![a.png](${makeImage().links.original})`);

    await dropdown.vm.$emit("select", "props");
    await flushPromises();
    expect(getImageExifMock).toHaveBeenCalledWith(1);
    expect(document.body.textContent).toContain("仅本人可见");

    await dropdown.vm.$emit("select", "toggle");
    await flushPromises();
    expect(setImageVisibilityMock).toHaveBeenCalledWith(1, true);
    vi.unstubAllGlobals();
  });

  it("右键菜单删除需经对话框确认", async () => {
    deleteImageMock.mockResolvedValue(null);
    const wrapper = await mountLibrary();
    await flushPromises();

    wrapper.findAllComponents(ImageCard)[0].vm.$emit("menu", new MouseEvent("contextmenu"));
    await flushPromises();
    await wrapper
      .findAllComponents(NDropdown)
      .find((node) => node.props("trigger") === "manual")!
      .vm.$emit("select", "remove");
    await flushPromises();
    expect(deleteImageMock).not.toHaveBeenCalled();

    await bodyButton("确认删除").click();
    await flushPromises();
    expect(deleteImageMock).toHaveBeenCalledWith(1);
  });

  it("挂载时加载相册列表供筛选与移动使用", async () => {
    await mountLibrary();
    await flushPromises();

    expect(listAlbumsMock).toHaveBeenCalledWith({ page: 1, size: 100 });
  });

  it("相册筛选切换后回到第 1 页并清空多选", async () => {
    const wrapper = await mountLibrary();
    await flushPromises();

    await wrapper.findAllComponents(ImageCard)[0].vm.$emit("select", true);
    await flushPromises();
    expect(wrapper.text()).toContain("已选 1 张");

    const selects = wrapper.findAllComponents(NSelect);
    await selects[0]!.vm.$emit("update:value", 7);
    await flushPromises();

    expect(listImagesMock).toHaveBeenLastCalledWith(params({ page: 1, size: 20, album_id: 7 }));
    expect(wrapper.text()).not.toContain("已选 1 张");
  });

  it("多选两张后批量移动：调用 batchAlbums 并刷新、清空选择", async () => {
    batchAlbumsMock.mockResolvedValue([batchItemOk(1), batchItemOk(2)]);
    const wrapper = await mountLibrary();
    await flushPromises();

    const cards = wrapper.findAllComponents(ImageCard);
    await cards[0].vm.$emit("select", true);
    await cards[1].vm.$emit("select", true);
    await flushPromises();
    expect(wrapper.text()).toContain("已选 2 张");

    const selects = wrapper.findAllComponents(NSelect);
    expect(selects.length).toBe(4);
    await selects[3]!.vm.$emit("update:value", 7);
    await flushPromises();

    await findButton(wrapper, "移动到相册").trigger("click");
    await flushPromises();

    expect(batchAlbumsMock).toHaveBeenCalledWith([1, 2], 7);
    expect(listImagesMock).toHaveBeenCalledTimes(2);
    expect(wrapper.text()).not.toContain("已选 2 张");
    expect(document.body.textContent).toContain("移动成功 2 张");
  });

  it("未选择目标相册时批量移动给出警告且不发请求", async () => {
    const wrapper = await mountLibrary();
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
    const wrapper = await mountLibrary();
    await flushPromises();

    const cards = wrapper.findAllComponents(ImageCard);
    await cards[0].vm.$emit("select", true);
    await cards[1].vm.$emit("select", true);
    const selects = wrapper.findAllComponents(NSelect);
    await selects[3]!.vm.$emit("update:value", 8);
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
    const wrapper = await mountLibrary();
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
    const wrapper = await mountLibrary();
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
    const wrapper = await mountLibrary();
    await flushPromises();

    await wrapper.findAllComponents(ImageCard)[0].vm.$emit("move", 7);
    await flushPromises();

    expect(batchAlbumsMock).toHaveBeenCalledWith([1], 7);
    expect(document.body.textContent).toContain("移动成功 1 张");
  });

  it("批量条「取消」清空选择", async () => {
    const wrapper = await mountLibrary();
    await flushPromises();

    await wrapper.findAllComponents(ImageCard)[0].vm.$emit("select", true);
    await flushPromises();
    expect(wrapper.text()).toContain("已选 1 张");

    await findButton(wrapper, "取消").trigger("click");
    await flushPromises();

    expect(wrapper.text()).not.toContain("已选 1 张");
  });

  it("changes all visible controls to English without remounting", async () => {
    const wrapper = mountLibrary();
    await flushPromises();
    i18n.global.locale.value = "en-US";
    await flushPromises();
    expect(wrapper.text()).toContain("42 images");
    expect(wrapper.text()).toContain("20 per page");
    expect(wrapper.text()).not.toContain("共 42 张图片");
  });

  it("ignores an old page result after newer navigation", async () => {
    let resolveOld!: (value: Awaited<ReturnType<typeof listImages>>) => void;
    listImagesMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveOld = resolve;
        }),
    );
    const wrapper = mountLibrary();
    wrapper.findComponent(NPagination).vm.$emit("update:page", 2);
    await flushPromises();
    resolveOld({ items: [makeImage({ id: 99, name: "stale.png" })], total: 1, page: 1, size: 20 });
    await flushPromises();
    expect(wrapper.text()).not.toContain("stale.png");
    expect(wrapper.text()).toContain("a.png");
  });

  it("does not duplicate a pending visibility mutation", async () => {
    setImageVisibilityMock.mockReturnValue(new Promise(() => {}));
    const wrapper = mountLibrary();
    await flushPromises();
    const card = wrapper.findComponent(ImageCard);
    card.vm.$emit("toggle", true);
    card.vm.$emit("toggle", true);
    await flushPromises();
    expect(setImageVisibilityMock).toHaveBeenCalledTimes(1);
  });

  it("deduplicates page-local selections", async () => {
    const wrapper = mountLibrary();
    await flushPromises();
    const card = wrapper.findComponent(ImageCard);
    card.vm.$emit("select", true);
    card.vm.$emit("select", true);
    await flushPromises();
    expect(wrapper.text()).toContain("已选 1 张");
  });

  it("does not keep previous-page images actionable when the new page fails and offers retry", async () => {
    const wrapper = mountLibrary();
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
    const wrapper = mountLibrary();
    await flushPromises();
    wrapper.findComponent(ImageCard).vm.$emit("toggle", true);
    await flushPromises();
    i18n.global.locale.value = "en-US";
    await flushPromises();
    expect(document.body.textContent).toContain("Could not change visibility");
    expect(document.body.textContent).not.toContain("可见性修改失败");
  });
});

it("drawer renders a large preview above the metadata", async () => {
  getImageExifMock.mockResolvedValue(makeExif({ image_id: 1 }));
  const wrapper = mount(
    () => h(NMessageProvider, () => h(NDialogProvider, () => h(ImageLibrary))),
    { attachTo: document.body },
  );
  await flushPromises();
  wrapper.findAllComponents(ImageCard)[0].vm.$emit("open");
  await flushPromises();
  expect(wrapper.findComponent(ImageDetailDrawer).exists()).toBe(true);
  expect(document.querySelector(".drawer-preview")).not.toBeNull();
  wrapper.unmount();
});
