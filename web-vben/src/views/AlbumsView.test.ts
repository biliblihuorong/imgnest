import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NMessageProvider, NPagination } from "naive-ui";
import { h, nextTick } from "vue";
import { i18n } from "@vben/locales";
import { createAlbum, deleteAlbum, listAlbums, updateAlbum } from "@/api/albums";
import { makeAlbum } from "@/components/albums/fixtures";
import AlbumCard from "@/components/albums/AlbumCard.vue";
import AlbumsView from "./AlbumsView.vue";

vi.mock("@/api/albums", () => ({
  listAlbums: vi.fn(),
  createAlbum: vi.fn(),
  updateAlbum: vi.fn(),
  deleteAlbum: vi.fn(),
}));

vi.mock("@/api/images", () => ({
  listImages: vi.fn(),
}));

const listAlbumsMock = vi.mocked(listAlbums);
const createAlbumMock = vi.mocked(createAlbum);
const updateAlbumMock = vi.mocked(updateAlbum);
const deleteAlbumMock = vi.mocked(deleteAlbum);

const { pushMock } = vi.hoisted(() => ({ pushMock: vi.fn() }));
vi.mock("vue-router", () => ({
  useRouter: () => ({ push: pushMock }),
}));

enableAutoUnmount(afterEach);

function mountAlbums() {
  return mount(() => h(NMessageProvider, () => h(AlbumsView)));
}

function viewButton(wrapper: Awaited<ReturnType<typeof mountAlbums>>, text: string) {
  const button = [...wrapper.findAll("button")].find((node) => node.text().trim() === text);
  if (!button) {
    throw new Error(`视图中找不到按钮：${text}`);
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

function setInputValue(element: HTMLInputElement, value: string): void {
  element.value = value;
  element.dispatchEvent(new Event("input"));
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
  i18n.global.locale.value = "zh-CN";
  pushMock.mockReset();
  vi.resetAllMocks();
  listAlbumsMock.mockResolvedValue({
    items: [
      makeAlbum({ id: 1, name: "旅行", image_count: 5, is_public: true }),
      makeAlbum({ id: 2, name: "工作", image_count: 0, is_public: false, cover_thumb_url: "" }),
    ],
    total: 2,
    page: 1,
    size: 20,
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("AlbumsView", () => {
  it("挂载后按 page=1&size=20 加载并渲染相册卡片", async () => {
    const wrapper = await mountAlbums();
    await flushPromises();

    expect(listAlbumsMock).toHaveBeenCalledWith({ page: 1, size: 20 });
    expect(wrapper.text()).toContain("共 2 个相册");
    expect(wrapper.text()).toContain("旅行");
    expect(wrapper.text()).toContain("5 张图片");
    expect(wrapper.findAllComponents(AlbumCard)).toHaveLength(2);
  });

  it("没有相册时空态按钮打开新建表单而不是上传页", async () => {
    listAlbumsMock.mockResolvedValue({ items: [], total: 0, page: 1, size: 20 });
    const wrapper = await mountAlbums();
    await flushPromises();

    expect(wrapper.text()).toContain("还没有相册");
    await wrapper.find(".albums-view__empty button").trigger("click");
    await flushPromises();
    expect(document.body.querySelector(".n-modal")).not.toBeNull();
    expect(document.body.querySelector(".n-modal")?.textContent).toContain("新建相册");
    expect(createAlbumMock).not.toHaveBeenCalled();
    expect(pushMock).not.toHaveBeenCalled();
    await bodyButton("取消").click();
    await flushPromises();
    await wrapper.find(".albums-view__empty button").trigger("click");
    await flushPromises();
    expect(document.body.querySelector<HTMLInputElement>(".n-modal input")?.value).toBe("");
  });

  it("已经存在的空相册仍显示相册卡片，不误显示无相册引导", async () => {
    listAlbumsMock.mockResolvedValue({
      items: [makeAlbum({ id: 2, name: "空相册", image_count: 0 })],
      total: 1,
      page: 1,
      size: 20,
    });
    const wrapper = mountAlbums();
    await flushPromises();
    expect(wrapper.findAllComponents(AlbumCard)).toHaveLength(1);
    expect(wrapper.text()).toContain("0 张图片");
    expect(wrapper.find(".albums-view__empty").exists()).toBe(false);
  });

  it("外部删除导致末页失效时仅回退一次，不误称没有相册", async () => {
    const wrapper = mountAlbums();
    await flushPromises();
    listAlbumsMock.mockResolvedValueOnce({ items: [], total: 21, page: 3, size: 20 });
    listAlbumsMock.mockResolvedValueOnce({
      items: [makeAlbum({ id: 21, name: "最后的相册" })],
      total: 21,
      page: 2,
      size: 20,
    });
    wrapper.findComponent(NPagination).vm.$emit("update:page", 3);
    await flushPromises();
    expect(listAlbumsMock).toHaveBeenLastCalledWith({ page: 2, size: 20 });
    expect(listAlbumsMock).toHaveBeenCalledTimes(3);
    expect(wrapper.text()).toContain("最后的相册");
    expect(wrapper.text()).not.toContain("还没有相册");
  });

  it("空页但总数非零时不显示创建第一个相册的空态", async () => {
    listAlbumsMock.mockResolvedValue({ items: [], total: 2, page: 1, size: 20 });
    const wrapper = mountAlbums();
    await flushPromises();
    expect(wrapper.text()).toContain("本页没有相册");
    expect(wrapper.text()).not.toContain("还没有相册");
  });

  it("新建相册：Modal 提交调用 createAlbum 并刷新列表", async () => {
    createAlbumMock.mockResolvedValue(makeAlbum({ id: 3, name: "新相册" }));
    const wrapper = await mountAlbums();
    await flushPromises();
    expect(listAlbumsMock).toHaveBeenCalledTimes(1);

    await viewButton(wrapper, "新建相册").trigger("click");
    await flushPromises();
    expect(document.body.textContent).toContain("新建相册");

    const input = document.body.querySelector<HTMLInputElement>(
      ".n-modal .n-form-item .n-input input",
    );
    expect(input).not.toBeNull();
    setInputValue(input!, "新相册");
    await bodyButton("创建").click();
    await flushPromises();

    expect(createAlbumMock).toHaveBeenCalledWith({
      name: "新相册",
      intro: "",
      is_public: false,
      cover_image_id: 0,
    });
    expect(listAlbumsMock).toHaveBeenCalledTimes(2);
  });

  it("编辑相册：表单回填并以 PATCH 语义提交", async () => {
    updateAlbumMock.mockResolvedValue(makeAlbum({ id: 1, name: "旅行改" }));
    const wrapper = await mountAlbums();
    await flushPromises();

    await viewButton(wrapper, "编辑").trigger("click");
    await flushPromises();

    const input = document.body.querySelector<HTMLInputElement>(
      ".n-modal .n-form-item .n-input input",
    );
    expect(input?.value).toBe("旅行");

    setInputValue(input!, "旅行改");
    await bodyButton("保存").click();
    await flushPromises();

    expect(updateAlbumMock).toHaveBeenCalledWith(1, {
      name: "旅行改",
      intro: "2026 年的旅途记录",
      is_public: true,
      cover_image_id: 0,
    });
  });

  it("删除相册：确认文案注明图片保留，确认后调用 deleteAlbum 并刷新", async () => {
    deleteAlbumMock.mockResolvedValue(null);
    const wrapper = await mountAlbums();
    await flushPromises();
    expect(listAlbumsMock).toHaveBeenCalledTimes(1);

    await viewButton(wrapper, "删除").trigger("click");
    await flushPromises();

    const panel = document.body.querySelector(".n-popconfirm__panel");
    expect(panel?.textContent).toContain("图片会保留");

    await bodyButton("确认删除").click();
    await flushPromises();

    expect(deleteAlbumMock).toHaveBeenCalledWith(1);
    expect(listAlbumsMock).toHaveBeenCalledTimes(2);
    expect(document.body.textContent).toContain("相册已删除");
  });

  it("翻页时携带新的 page 参数", async () => {
    listAlbumsMock.mockResolvedValue({ items: [], total: 0, page: 1, size: 20 });
    const wrapper = await mountAlbums();
    await flushPromises();

    const pagination = wrapper.findComponent(NPagination);
    pagination.vm.$emit("update:page", 2);
    await flushPromises();

    expect(listAlbumsMock).toHaveBeenLastCalledWith({ page: 2, size: 20 });
  });

  it("ignores an older page response after navigation", async () => {
    let resolveFirst!: (value: Awaited<ReturnType<typeof listAlbums>>) => void;
    listAlbumsMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveFirst = resolve;
        }),
    );
    const wrapper = mountAlbums();
    listAlbumsMock.mockResolvedValueOnce({
      items: [makeAlbum({ id: 22, name: "Latest album" })],
      total: 30,
      page: 2,
      size: 20,
    });
    wrapper.findComponent(NPagination).vm.$emit("update:page", 2);
    await flushPromises();
    resolveFirst({ items: [makeAlbum({ name: "Stale album" })], total: 30, page: 1, size: 20 });
    await flushPromises();
    expect(wrapper.text()).toContain("Latest album");
    expect(wrapper.text()).not.toContain("Stale album");
    expect(wrapper.findComponent(NPagination).props("page")).toBe(2);
  });

  it("does not send duplicate delete requests while an album is busy", async () => {
    let finish!: (value: null) => void;
    deleteAlbumMock.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const wrapper = mountAlbums();
    await flushPromises();
    const card = wrapper.findComponent(AlbumCard);
    card.vm.$emit("remove");
    card.vm.$emit("remove");
    expect(deleteAlbumMock).toHaveBeenCalledTimes(1);
    finish(null);
    await flushPromises();
  });

  it("updates visible page and card labels when the language changes", async () => {
    const wrapper = mountAlbums();
    await flushPromises();
    i18n.global.locale.value = "en-US";
    await nextTick();
    expect(wrapper.text()).toContain("New album");
    expect(wrapper.text()).toContain("2 albums");
    expect(wrapper.text()).toContain("Private");
    expect(wrapper.text()).not.toContain("新建相册");
  });

  it("removes stale album rows and offers retry if a new page fails", async () => {
    const wrapper = mountAlbums();
    await flushPromises();
    listAlbumsMock.mockRejectedValueOnce(new Error("数据库错误"));
    wrapper.findComponent(NPagination).vm.$emit("update:page", 2);
    await flushPromises();
    expect(wrapper.findAllComponents(AlbumCard)).toHaveLength(0);
    expect(wrapper.text()).toContain("相册列表加载失败");
    expect(wrapper.text()).not.toContain("数据库错误");
    listAlbumsMock.mockResolvedValueOnce({
      items: [makeAlbum({ name: "Recovered" })],
      total: 30,
      page: 2,
      size: 20,
    });
    await viewButton(wrapper, "重试").trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("Recovered");
  });

  it("keeps the current page when an album deletion from a previous page finishes", async () => {
    let finish!: (value: null) => void;
    deleteAlbumMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const wrapper = mountAlbums();
    await flushPromises();
    wrapper.findComponent(AlbumCard).vm.$emit("remove");
    listAlbumsMock.mockResolvedValue({
      items: [makeAlbum({ id: 21, name: "Second page" })],
      total: 21,
      page: 2,
      size: 20,
    });
    wrapper.findComponent(NPagination).vm.$emit("update:page", 2);
    await flushPromises();
    finish(null);
    await flushPromises();
    expect(listAlbumsMock).toHaveBeenLastCalledWith({ page: 2, size: 20 });
    expect(wrapper.findComponent(NPagination).props("page")).toBe(2);
  });
});
