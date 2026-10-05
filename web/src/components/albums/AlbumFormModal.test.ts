import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NMessageProvider } from "naive-ui";
import { defineComponent, h } from "vue";
import { createAlbum, updateAlbum, type AlbumView } from "@/api/albums";
import { listImages } from "@/api/images";
import { makeImage } from "@/components/images/fixtures";
import { makeAlbum } from "./fixtures";
import AlbumFormModal from "./AlbumFormModal.vue";

vi.mock("@/api/albums", () => ({
  createAlbum: vi.fn(),
  updateAlbum: vi.fn(),
}));

vi.mock("@/api/images", () => ({
  listImages: vi.fn(),
}));

const createAlbumMock = vi.mocked(createAlbum);
const updateAlbumMock = vi.mocked(updateAlbum);
const listImagesMock = vi.mocked(listImages);

enableAutoUnmount(afterEach);

function mountModal(album: AlbumView | null = null) {
  const host = defineComponent({
    render: () => h(NMessageProvider, () => h(AlbumFormModal, { show: true, album })),
  });
  return mount(host);
}

/** NModal 内容 teleport 到 body，事件断言取子组件实例。 */
function modalEmitted(wrapper: Awaited<ReturnType<typeof mountModal>>, event: string) {
  return wrapper.findComponent(AlbumFormModal).emitted(event);
}

function setInputValue(element: HTMLInputElement | HTMLTextAreaElement, value: string): void {
  element.value = value;
  element.dispatchEvent(new Event("input"));
}

function bodyInput(): HTMLInputElement {
  const input = document.body.querySelector<HTMLInputElement>(".n-modal .n-form-item .n-input input");
  if (!input) {
    throw new Error("找不到名称输入框");
  }
  return input;
}

function bodyTextarea(): HTMLTextAreaElement {
  const textarea = document.body.querySelector<HTMLTextAreaElement>(".n-modal textarea");
  if (!textarea) {
    throw new Error("找不到简介输入框");
  }
  return textarea;
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

async function fillAndSubmit(name: string, intro = "简介内容"): Promise<void> {
  setInputValue(bodyInput(), name);
  setInputValue(bodyTextarea(), intro);
  await bodyButton("创建").click();
  await flushPromises();
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
  listImagesMock.mockResolvedValue({ items: [], total: 0, page: 1, size: 12 });
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.resetAllMocks();
});

describe("AlbumFormModal", () => {
  it("新建模式：名称必填，空名称提交不发请求并就地提示", async () => {
    const wrapper = mountModal();

    await bodyButton("创建").click();
    await flushPromises();

    expect(createAlbumMock).not.toHaveBeenCalled();
    expect(modalEmitted(wrapper, "saved")).toBeUndefined();
    expect(document.body.textContent).toContain("请输入相册名称");
  });

  it("新建模式：填写后提交调用 createAlbum 并发出 saved 与关闭事件", async () => {
    const savedAlbum = makeAlbum({ id: 9, name: "旅行" });
    createAlbumMock.mockResolvedValue(savedAlbum);
    const wrapper = mountModal();

    await fillAndSubmit("旅行");

    expect(createAlbumMock).toHaveBeenCalledWith({
      name: "旅行",
      intro: "简介内容",
      is_public: false,
      cover_image_id: 0,
    });
    expect(modalEmitted(wrapper, "saved")).toEqual([[savedAlbum]]);
    expect(modalEmitted(wrapper, "update:show")).toEqual([[false]]);
  });

  it("编辑模式：回填字段并以 PATCH 语义调用 updateAlbum", async () => {
    const album = makeAlbum({ id: 3, name: "旧名字", intro: "旧简介", is_public: true });
    updateAlbumMock.mockResolvedValue(makeAlbum({ id: 3, name: "新名字" }));
    const wrapper = mountModal(album);
    await flushPromises();

    expect(document.body.querySelector(".n-modal .n-card-header__main")?.textContent).toContain(
      "编辑相册",
    );
    expect(bodyInput().value).toBe("旧名字");
    expect(bodyTextarea().value).toBe("旧简介");

    setInputValue(bodyInput(), "新名字");
    await bodyButton("保存").click();
    await flushPromises();

    expect(updateAlbumMock).toHaveBeenCalledWith(3, {
      name: "新名字",
      intro: "旧简介",
      is_public: true,
      cover_image_id: 0,
    });
    expect(createAlbumMock).not.toHaveBeenCalled();
    expect(modalEmitted(wrapper, "saved")).toHaveLength(1);
  });

  it("封面：从我的图片选择里单选，提交携带 cover_image_id", async () => {
    listImagesMock.mockResolvedValue({
      items: [
        makeImage({ id: 7, name: "a.png", local_thumb_url: "/t/a.webp" }),
        makeImage({ id: 8, name: "b.jpg", local_thumb_url: "/t/b.webp" }),
      ],
      total: 2,
      page: 1,
      size: 12,
    });
    createAlbumMock.mockResolvedValue(makeAlbum({ id: 1 }));
    const wrapper = mountModal();

    await bodyButton("从我的图片选择").click();
    await flushPromises();

    expect(listImagesMock).toHaveBeenCalledWith({ page: 1, size: 12 });
    const items = document.body.querySelectorAll(".album-form__picker-item");
    expect(items).toHaveLength(2);

    items[1].dispatchEvent(new MouseEvent("click", { bubbles: true }));
    await flushPromises();
    expect(document.body.textContent).toContain("图片 #8");

    await fillAndSubmit("带封面");
    expect(createAlbumMock).toHaveBeenCalledWith({
      name: "带封面",
      intro: "简介内容",
      is_public: false,
      cover_image_id: 8,
    });
    expect(modalEmitted(wrapper, "saved")).toHaveLength(1);
  });

  it("封面：清除封面后回到未设置并提交 0", async () => {
    listImagesMock.mockResolvedValue({
      items: [makeImage({ id: 7, name: "a.png", local_thumb_url: "/t/a.webp" })],
      total: 1,
      page: 1,
      size: 12,
    });
    createAlbumMock.mockResolvedValue(makeAlbum({ id: 1 }));
    const wrapper = mountModal();

    await bodyButton("从我的图片选择").click();
    await flushPromises();
    const items = document.body.querySelectorAll(".album-form__picker-item");
    items[0].dispatchEvent(new MouseEvent("click", { bubbles: true }));
    await flushPromises();
    expect(document.body.textContent).toContain("图片 #7");

    await bodyButton("清除封面").click();
    expect(document.body.textContent).toContain("未设置");

    await fillAndSubmit("无封面");
    expect(createAlbumMock).toHaveBeenCalledWith({
      name: "无封面",
      intro: "简介内容",
      is_public: false,
      cover_image_id: 0,
    });
    expect(modalEmitted(wrapper, "saved")).toHaveLength(1);
  });

  it("创建失败：提示错误且不关闭弹窗", async () => {
    createAlbumMock.mockRejectedValue(new Error("名称已存在"));
    const wrapper = mountModal();

    await fillAndSubmit("旅行");

    expect(modalEmitted(wrapper, "update:show")).toBeUndefined();
    expect(document.body.textContent).toContain("名称已存在");
  });
});
