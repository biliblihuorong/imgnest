import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NMessageProvider } from "naive-ui";
import { h, nextTick, ref } from "vue";
import { i18n } from "@vben/locales";
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
  return mount(() => h(NMessageProvider, () => h(AlbumFormModal, { show: true, album })));
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
  const input = document.body.querySelector<HTMLInputElement>(
    ".n-modal .n-form-item .n-input input",
  );
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
  i18n.global.locale.value = "zh-CN";
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe(): void {}
      unobserve(): void {}
      disconnect(): void {}
    },
  );
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockImplementation(() =>
        Promise.resolve(new Response(new Blob(["private thumbnail"], { type: "image/webp" }))),
      ),
  );
  URL.createObjectURL = vi.fn().mockReturnValue("blob:album-option");
  URL.revokeObjectURL = vi.fn();
  localStorage.clear();
  listImagesMock.mockResolvedValue({ items: [], total: 0, page: 1, size: 12 });
});

afterEach(() => {
  localStorage.clear();
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
      size: 24,
    });
    createAlbumMock.mockResolvedValue(makeAlbum({ id: 1 }));
    const wrapper = mountModal();

    await bodyButton("从我的图片选择").click();
    await flushPromises();

    expect(listImagesMock).toHaveBeenCalledWith({ page: 1, size: 24 });
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
      size: 24,
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
    expect(document.body.textContent).toContain("保存相册失败");
    expect(document.body.textContent).not.toContain("名称已存在");
  });

  it("locks submission before asynchronous validation to prevent repeated creates", async () => {
    let finish!: (value: AlbumView) => void;
    createAlbumMock.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    mountModal();
    setInputValue(bodyInput(), "One album");
    await nextTick();
    const button = bodyButton("创建");
    button.click();
    button.click();
    await flushPromises();
    expect(createAlbumMock).toHaveBeenCalledTimes(1);
    finish(makeAlbum());
    await flushPromises();
  });

  it("counts Unicode code points so a 100-emoji album name is accepted", async () => {
    createAlbumMock.mockResolvedValue(makeAlbum());
    mountModal();
    await fillAndSubmit("🖼".repeat(100), "");
    expect(createAlbumMock).toHaveBeenCalledWith(
      expect.objectContaining({ name: "🖼".repeat(100) }),
    );
  });

  it("rejects a 101-code-point name and a 501-code-point introduction", async () => {
    mountModal();
    await fillAndSubmit("x".repeat(101), "y".repeat(501));
    expect(createAlbumMock).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("名称不能超过 100 个字符");
    expect(document.body.textContent).toContain("简介不能超过 500 个字符");
  });

  it("updates labels and displayed validation errors on a live language switch", async () => {
    mountModal();
    bodyButton("创建").click();
    await flushPromises();
    expect(document.body.textContent).toContain("请输入相册名称");
    i18n.global.locale.value = "en-US";
    await flushPromises();
    expect(document.body.textContent).toContain("New album");
    expect(document.body.textContent).toContain("Enter an album name");
    expect(document.body.textContent).not.toContain("请输入相册名称");
  });

  it("does not let a cancelled picker request replace images in a reopened dialog", async () => {
    let finishOlder!: (value: Awaited<ReturnType<typeof listImages>>) => void;
    listImagesMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finishOlder = resolve;
        }),
    );
    const show = ref(true);
    mount(() =>
      h(NMessageProvider, () =>
        h(AlbumFormModal, {
          show: show.value,
          album: null,
          "onUpdate:show": (value: boolean) => {
            show.value = value;
          },
        }),
      ),
    );
    bodyButton("从我的图片选择").click();
    await flushPromises();
    bodyButton("取消").click();
    await flushPromises();
    show.value = true;
    await flushPromises();
    listImagesMock.mockResolvedValueOnce({
      items: [makeImage({ id: 2, name: "Current choice" })],
      total: 1,
      page: 1,
      size: 24,
    });
    bodyButton("从我的图片选择").click();
    await flushPromises();
    finishOlder({
      items: [makeImage({ id: 1, name: "Stale choice" })],
      total: 1,
      page: 1,
      size: 24,
    });
    await flushPromises();
    expect(document.body.querySelector(".album-form__picker-item")?.getAttribute("title")).toBe(
      "Current choice",
    );
  });

  it("uses authenticated blob thumbnails in the picker and releases them when cancelled", async () => {
    localStorage.setItem("imgnest.token", "picker-secret");
    listImagesMock.mockResolvedValue({
      items: [
        makeImage({ id: 2, name: "Private choice", local_thumb_url: "/t/private-choice.webp" }),
      ],
      total: 1,
      page: 1,
      size: 24,
    });
    mountModal();
    bodyButton("从我的图片选择").click();
    await flushPromises();
    const image = document.body.querySelector(".album-form__picker-item img");
    expect(image?.getAttribute("src")).toBe("blob:album-option");
    expect(document.body.innerHTML).not.toContain("picker-secret");
    const [url, init] = vi.mocked(fetch).mock.calls[0]!;
    expect(url).toBe("/t/private-choice.webp");
    expect(new Headers(init?.headers).get("Authorization")).toBe("Bearer picker-secret");
    bodyButton("取消").click();
    await flushPromises();
    expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:album-option");
  });

  it("does not close a reopened form when an earlier save completes", async () => {
    let finish!: (value: AlbumView) => void;
    createAlbumMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const show = ref(true);
    const wrapper = mount(() =>
      h(NMessageProvider, () => h(AlbumFormModal, { show: show.value, album: null })),
    );
    await fillAndSubmit("Old request");
    show.value = false;
    await nextTick();
    show.value = true;
    await flushPromises();
    setInputValue(bodyInput(), "New draft");
    finish(makeAlbum({ name: "Old request" }));
    await flushPromises();
    expect(bodyInput().value).toBe("New draft");
    expect(modalEmitted(wrapper, "saved")).toBeUndefined();
    expect(modalEmitted(wrapper, "update:show")).toBeUndefined();
  });

  it("shows an English fallback instead of a Chinese backend diagnostic", async () => {
    i18n.global.locale.value = "en-US";
    createAlbumMock.mockRejectedValue(new Error("数据库详细错误"));
    mountModal();
    setInputValue(bodyInput(), "Summer");
    bodyButton("Create").click();
    await flushPromises();
    expect(document.body.textContent).toContain("Could not save the album");
    expect(document.body.textContent).not.toContain("数据库详细错误");
  });

  it("translates a blur validation error before the form has been submitted", async () => {
    mountModal();
    bodyInput().dispatchEvent(new Event("blur"));
    await flushPromises();
    expect(document.body.textContent).toContain("请输入相册名称");
    i18n.global.locale.value = "en-US";
    await flushPromises();
    expect(document.body.textContent).toContain("Enter an album name");
    expect(document.body.textContent).not.toContain("请输入相册名称");
  });
});
