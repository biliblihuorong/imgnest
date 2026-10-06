import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NDialogProvider, NDropdown, NMessageProvider, NPagination, NSelect } from "naive-ui";
import { h, ref } from "vue";
import { i18n } from "@vben/locales";
import { listAlbums } from "@/api/albums";
import {
  batchAlbums,
  batchDelete,
  batchPermission,
  deleteImage,
  getImageExif,
  searchImages,
  setImageVisibility,
  type ImageSearchParams,
} from "@/api/images";
import { makeAlbum } from "@/components/albums/fixtures";
import { fetchProtectedThumbnail } from "@/api/thumbnails";
import ImageCard from "@/components/images/ImageCard.vue";
import ImageDetailDrawer from "@/components/images/ImageDetailDrawer.vue";
import { makeExif, makeImage } from "@/components/images/fixtures";
import ImageDetailPanel from "@/components/images/ImageDetailPanel.vue";
import { setShell } from "@/integrations/shell/useShell";
import ImageLibrary from "./ImageLibrary.vue";

// jsdom 没有 matchMedia，默认按窄屏处理（沿用详情抽屉）；面板用例显式置为宽屏。
const wideViewport = ref(false);
vi.mock("@/components/layout/marvis/useNarrowViewport", () => ({
  useViewportAtLeast: () => wideViewport,
}));

vi.mock("@/api/images", () => ({
  searchImages: vi.fn(),
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
  suggestAlbums: vi.fn().mockResolvedValue({ items: [], hasMore: false }),
}));

vi.mock("@/api/thumbnails", () => ({
  fetchProtectedThumbnail: vi.fn(),
}));

const searchImagesMock = vi.mocked(searchImages);
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
function params(overrides: Partial<ImageSearchParams> = {}): ImageSearchParams {
  return { qv: 1, q: "", tz: "UTC", page: 1, size: 20, ...overrides };
}

function mountLibrary(props: Record<string, unknown> = {}) {
  return mount(() => h(NMessageProvider, () => h(NDialogProvider, () => h(ImageLibrary, props))), {
    attachTo: document.body,
  });
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

function searchMetadata(q = "") {
  return {
    appliedVersion: 1 as const,
    canonicalQ: q,
    tz: "UTC",
    authorizedAlbums: [],
    appliedRange: { afterUtc: null, beforeUtc: null },
  };
}

async function submitQuery(wrapper: ReturnType<typeof mountLibrary>, raw: string) {
  await wrapper.get(".unified-search__input").setValue(raw);
  await wrapper.get(".unified-search__form").trigger("submit");
  await flushPromises();
}

function batchItemOk(id: number) {
  return { id, status: 200, code: 0, message: "ok", data: null };
}

beforeEach(() => {
  vi.resetAllMocks();
  searchImagesMock.mockImplementation(async (p) => ({
    items: [
      makeImage({ id: 1, name: "a.png", size: 2048 }),
      makeImage({ id: 2, name: "b.jpg", is_public: true }),
    ],
    total: 42,
    page: p.page,
    size: p.size,
    search: searchMetadata(p.q),
  }));
  listAlbumsMock.mockResolvedValue({
    items: [makeAlbum({ id: 7, name: "旅行" }), makeAlbum({ id: 8, name: "工作" })],
    total: 2,
    page: 1,
    size: 100,
  });
  URL.createObjectURL = vi.fn().mockReturnValue("blob:library-thumb");
  URL.revokeObjectURL = vi.fn();
  fetchProtectedThumbnailMock.mockResolvedValue(new Blob(["image"], { type: "image/webp" }));
});

describe("ImageLibrary", () => {
  it("mounts a versioned search and retains image grid and count", async () => {
    const wrapper = mountLibrary();
    await flushPromises();
    expect(searchImagesMock).toHaveBeenCalledWith(
      params({ tz: Intl.DateTimeFormat().resolvedOptions().timeZone }),
      expect.any(AbortSignal),
      undefined,
    );
    expect(wrapper.text()).toContain("共 42 张匹配图片");
    expect(wrapper.findAllComponents(ImageCard)).toHaveLength(2);
  });
  it("shows real empty results separately from errors", async () => {
    searchImagesMock.mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      size: 20,
      search: searchMetadata(),
    });
    const wrapper = mountLibrary();
    await flushPromises();
    expect(wrapper.text()).toContain("还没有图片");
  });
  it("pagination retains the versioned query and resets on size changes", async () => {
    const wrapper = mountLibrary();
    await flushPromises();
    wrapper.findComponent(NPagination).vm.$emit("update:page", 3);
    await flushPromises();
    expect(searchImagesMock).toHaveBeenLastCalledWith(
      params({ page: 3 }),
      expect.any(AbortSignal),
      undefined,
    );
    await wrapper.findComponent(NSelect).vm.$emit("update:value", 50);
    await flushPromises();
    expect(searchImagesMock).toHaveBeenLastCalledWith(
      params({ page: 1, size: 50 }),
      expect.any(AbortSignal),
      undefined,
    );
  });
  it("the single draft supports album, format, camera, size, date, visibility and sort", async () => {
    const wrapper = mountLibrary();
    await flushPromises();
    expect(wrapper.findAllComponents(NSelect)).toHaveLength(1);
    await submitQuery(
      wrapper,
      "album:#7 format:jpeg camera:Canon minsize:1MB maxsize:5MB after:2026-10-01 before:2026-11-01 visibility:private order:utmost",
    );
    expect(searchImagesMock).toHaveBeenLastCalledWith(
      params({
        q: "album:#7 format:jpeg camera:Canon minsize:1MB maxsize:5MB after:2026-10-01 before:2026-11-01 visibility:private order:utmost",
      }),
      expect.any(AbortSignal),
      undefined,
    );
  });
  it("typing remains local and clear submits an empty query", async () => {
    const wrapper = mountLibrary();
    await flushPromises();
    await wrapper.get(".unified-search__input").setValue("vacation");
    await flushPromises();
    expect(searchImagesMock).toHaveBeenCalledTimes(1);
    await submitQuery(wrapper, "vacation");
    expect(searchImagesMock).toHaveBeenCalledTimes(2);
    await findButton(wrapper, "清空").trigger("click");
    await flushPromises();
    expect(searchImagesMock).toHaveBeenLastCalledWith(params(), expect.any(AbortSignal), undefined);
  });
  it("fixed album remains a visibly separate scope even when clearing query", async () => {
    const wrapper = mountLibrary({ lockedAlbumId: 7, lockedAlbumName: "旅行" });
    await flushPromises();
    expect(searchImagesMock).toHaveBeenCalledWith(
      params({ tz: Intl.DateTimeFormat().resolvedOptions().timeZone }),
      expect.any(AbortSignal),
      7,
    );
    expect(wrapper.get('[data-testid="fixed-album-scope"]').text()).toContain("旅行");
    await submitQuery(wrapper, "album:#8");
    await findButton(wrapper, "清空").trigger("click");
    await flushPromises();
    expect(searchImagesMock).toHaveBeenLastCalledWith(params(), expect.any(AbortSignal), 7);
  });

  it("可见性切换成功：调用 PATCH 并更新列表项", async () => {
    setImageVisibilityMock.mockResolvedValue(makeImage({ id: 1, is_public: true }));
    const wrapper = await mountLibrary();
    await flushPromises();

    searchImagesMock.mockResolvedValueOnce({
      items: [makeImage({ id: 1, is_public: true }), makeImage({ id: 2, is_public: true })],
      total: 42,
      page: 1,
      size: 20,
      search: searchMetadata(),
    });
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
    expect(searchImagesMock).toHaveBeenCalledTimes(1);

    wrapper.findAllComponents(ImageCard)[0].vm.$emit("remove");
    await flushPromises();

    expect(deleteImageMock).toHaveBeenCalledWith(1);
    expect(searchImagesMock).toHaveBeenCalledTimes(2);
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

  it("submitting a new query clears page-local selections", async () => {
    const wrapper = mountLibrary();
    await flushPromises();
    await wrapper.findComponent(ImageCard).vm.$emit("select", true);
    await flushPromises();
    expect(wrapper.text()).toContain("已选 1 张");
    await submitQuery(wrapper, "album:#7");
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
    expect(selects.length).toBe(2);
    await selects[1]!.vm.$emit("update:value", 7);
    await flushPromises();

    await findButton(wrapper, "移动到相册").trigger("click");
    await flushPromises();

    expect(batchAlbumsMock).toHaveBeenCalledWith([1, 2], 7);
    expect(searchImagesMock).toHaveBeenCalledTimes(2);
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
    await selects[1]!.vm.$emit("update:value", 8);
    await flushPromises();

    await findButton(wrapper, "移动到相册").trigger("click");
    await flushPromises();

    expect(document.body.textContent).toContain("移动成功 1 张");
    expect(document.body.textContent).toContain("移动失败（ID 1）：记录不存在或已被删除");
    expect(searchImagesMock).toHaveBeenCalledTimes(2);
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
    expect(wrapper.text()).toContain("42 matching images");
    expect(wrapper.text()).toContain("20 per page");
    expect(wrapper.text()).not.toContain("共 42 张匹配图片");
  });

  it("ignores an old page result after newer navigation", async () => {
    let resolveOld!: (value: Awaited<ReturnType<typeof searchImages>>) => void;
    searchImagesMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveOld = resolve;
        }),
    );
    const wrapper = mountLibrary();
    wrapper.findComponent(NPagination).vm.$emit("update:page", 2);
    await flushPromises();
    resolveOld({
      items: [makeImage({ id: 99, name: "stale.png" })],
      total: 1,
      page: 1,
      size: 20,
      search: searchMetadata(),
    });
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
    searchImagesMock.mockRejectedValueOnce(new Error("offline"));
    wrapper.findComponent(NPagination).vm.$emit("update:page", 2);
    await flushPromises();
    expect(wrapper.findAllComponents(ImageCard)).toHaveLength(2);
    expect(wrapper.findComponent(ImageCard).props("busy")).toBe(true);
    expect(wrapper.text()).toContain("显示上次成功结果");
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

it("single visibility mutation reloads applied filters and count without applying an unsent draft", async () => {
  const wrapper = mountLibrary();
  await flushPromises();
  await submitQuery(wrapper, "visibility:public");
  await wrapper.get(".unified-search__input").setValue("camera:unsent");
  setImageVisibilityMock.mockResolvedValue(makeImage({ id: 1, is_public: false }));
  searchImagesMock.mockResolvedValueOnce({
    items: [],
    total: 0,
    page: 1,
    size: 20,
    search: searchMetadata("visibility:public"),
  });
  wrapper.findComponent(ImageCard).vm.$emit("toggle", false);
  await flushPromises();
  expect(searchImagesMock.mock.calls.at(-1)?.[0].q).toBe("visibility:public");
  expect(wrapper.findAllComponents(ImageCard)).toHaveLength(0);
  expect(wrapper.text()).toContain("共 0 张匹配图片");
  expect(wrapper.get<HTMLInputElement>(".unified-search__input").element.value).toBe(
    "camera:unsent",
  );
});

describe("ImageLibrary 详情面板（Marvis 宽屏）", () => {
  beforeEach(() => {
    getImageExifMock.mockResolvedValue(makeExif({ image_id: 1 }));
    wideViewport.value = true;
    setShell("marvis");
  });
  afterEach(() => {
    wideViewport.value = false;
    setShell("marvis");
  });

  it("uses the side panel instead of the drawer", async () => {
    const wrapper = mountLibrary();
    await flushPromises();
    expect(wrapper.findComponent(ImageDetailPanel).exists()).toBe(true);
    expect(wrapper.findComponent(ImageDetailDrawer).exists()).toBe(false);
    expect(wrapper.findComponent(ImageDetailPanel).props("image")).toBeNull();
  });

  it.each([
    ["classic", true],
    ["marvis", false],
  ] as const)("keeps the drawer for shell=%s wide=%s", async (shellName, wide) => {
    setShell(shellName);
    wideViewport.value = wide;
    const wrapper = mountLibrary();
    await flushPromises();
    expect(wrapper.findComponent(ImageDetailDrawer).exists()).toBe(true);
    expect(wrapper.findComponent(ImageDetailPanel).exists()).toBe(false);
  });

  it("shows the opened image in the panel, highlights its card and closes again", async () => {
    const wrapper = mountLibrary();
    await flushPromises();
    const cards = wrapper.findAllComponents(ImageCard);
    cards[0].vm.$emit("open");
    await flushPromises();
    const panel = wrapper.findComponent(ImageDetailPanel);
    expect(panel.props("image")).toMatchObject({ id: 1, name: "a.png" });
    expect(getImageExifMock).toHaveBeenCalledWith(1);
    expect(cards[0].classes()).toContain("image-card--active");
    expect(cards[1].classes()).not.toContain("image-card--active");
    panel.vm.$emit("close");
    await flushPromises();
    expect(panel.props("image")).toBeNull();
    expect(cards[0].classes()).not.toContain("image-card--active");
  });

  it("selects a card when its info area is clicked, but not when a control is clicked", async () => {
    setImageVisibilityMock.mockResolvedValue(makeImage({ id: 2, is_public: false }));
    const wrapper = mountLibrary();
    await flushPromises();
    const cards = wrapper.findAllComponents(ImageCard);
    await cards[1].find(".n-switch").trigger("click");
    await flushPromises();
    expect(wrapper.findComponent(ImageDetailPanel).props("image")).toBeNull();
    await cards[1].find(".image-card__meta").trigger("click");
    await flushPromises();
    expect(wrapper.findComponent(ImageDetailPanel).props("image")).toMatchObject({ id: 2 });
  });

  it("keeps showing the same image after the viewport narrows", async () => {
    const wrapper = mountLibrary();
    await flushPromises();
    wrapper.findAllComponents(ImageCard)[0].vm.$emit("open");
    await flushPromises();
    wideViewport.value = false;
    await flushPromises();
    expect(wrapper.findComponent(ImageDetailPanel).exists()).toBe(false);
    const drawer = wrapper.findComponent(ImageDetailDrawer);
    expect(drawer.props("show")).toBe(true);
    expect(drawer.props("image")).toMatchObject({ id: 1 });
  });

  it("offers the same context-menu options and copy results as the drawer mode", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal("navigator", { userAgent: "vitest", clipboard: { writeText } });
    const wrapper = mountLibrary();
    await flushPromises();
    wrapper.findAllComponents(ImageCard)[0].vm.$emit("menu", new MouseEvent("contextmenu"));
    await flushPromises();
    const dropdown = wrapper
      .findAllComponents(NDropdown)
      .find((node) => node.props("trigger") === "manual")!;
    const keys = (dropdown.props("options") as { key: string }[]).map((option) => option.key);
    expect(keys).toEqual([
      "copy-original",
      "copy-webp",
      "copy-thumbnail",
      "divider",
      "props",
      "toggle",
      "remove",
    ]);
    await dropdown.vm.$emit("select", "copy-original:bbcode");
    await flushPromises();
    expect(writeText).toHaveBeenCalledWith(`[img]${makeImage().links.original}[/img]`);
    await dropdown.vm.$emit("select", "props");
    await flushPromises();
    expect(wrapper.findComponent(ImageDetailPanel).props("image")).toMatchObject({ id: 1 });
    vi.unstubAllGlobals();
  });

  it("clears the panel when the shown image is no longer in the list", async () => {
    const wrapper = mountLibrary();
    await flushPromises();
    wrapper.findAllComponents(ImageCard)[0].vm.$emit("open");
    await flushPromises();
    expect(wrapper.findComponent(ImageDetailPanel).props("image")).toMatchObject({ id: 1 });
    searchImagesMock.mockImplementation(async (p) => ({
      items: [makeImage({ id: 2, name: "b.jpg", is_public: true })],
      total: 1,
      page: p.page,
      size: p.size,
      search: searchMetadata(p.q),
    }));
    deleteImageMock.mockResolvedValue(null);
    wrapper.findAllComponents(ImageCard)[0].vm.$emit("remove");
    await flushPromises();
    expect(wrapper.findAllComponents(ImageCard)).toHaveLength(1);
    expect(wrapper.findComponent(ImageDetailPanel).props("image")).toBeNull();
  });

  it("shows the refreshed copy of the image after it is updated in place", async () => {
    setImageVisibilityMock.mockResolvedValue(makeImage({ id: 1, name: "a.png", is_public: true }));
    const wrapper = mountLibrary();
    await flushPromises();
    wrapper.findAllComponents(ImageCard)[0].vm.$emit("open");
    await flushPromises();
    searchImagesMock.mockImplementation(async (p) => ({
      items: [makeImage({ id: 1, name: "a.png", is_public: true })],
      total: 1,
      page: p.page,
      size: p.size,
      search: searchMetadata(p.q),
    }));
    wrapper.findAllComponents(ImageCard)[0].vm.$emit("toggle", true);
    await flushPromises();
    expect(wrapper.findComponent(ImageDetailPanel).props("image")).toMatchObject({
      id: 1,
      is_public: true,
    });
    expect(getImageExifMock).toHaveBeenCalledTimes(1);
  });

  it("card click in drawer mode does not open details from the info area", async () => {
    wideViewport.value = false;
    const wrapper = mountLibrary();
    await flushPromises();
    await wrapper.findAllComponents(ImageCard)[0].find(".image-card__meta").trigger("click");
    await flushPromises();
    expect(wrapper.findComponent(ImageDetailDrawer).props("show")).toBe(false);
  });
});
