import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NMessageProvider, NPagination } from "naive-ui";
import { h } from "vue";
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

  it("列表为空时显示空态，引导按钮跳转上传页", async () => {
    listAlbumsMock.mockResolvedValue({ items: [], total: 0, page: 1, size: 20 });
    const wrapper = await mountAlbums();
    await flushPromises();

    expect(wrapper.text()).toContain("还没有相册");
    await viewButton(wrapper, "去上传页").trigger("click");

    expect(pushMock).toHaveBeenCalledWith("/upload");
  });

  it("新建相册：Modal 提交调用 createAlbum 并刷新列表", async () => {
    createAlbumMock.mockResolvedValue(makeAlbum({ id: 3, name: "新相册" }));
    const wrapper = await mountAlbums();
    await flushPromises();
    expect(listAlbumsMock).toHaveBeenCalledTimes(1);

    await viewButton(wrapper, "新建相册").trigger("click");
    await flushPromises();
    expect(document.body.textContent).toContain("新建相册");

    const input = document.body.querySelector<HTMLInputElement>(".n-modal .n-form-item .n-input input");
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

    const input = document.body.querySelector<HTMLInputElement>(".n-modal .n-form-item .n-input input");
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
});
