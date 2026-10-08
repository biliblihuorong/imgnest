import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NMessageProvider } from "naive-ui";
import { h, ref } from "vue";
import { i18n } from "@vben/locales";
import {
  deleteRandomLink,
  getRandomLink,
  putRandomLink,
  resetRandomLink,
  type RandomLinkView,
} from "@/api/albums";
import AlbumRandomLink from "./AlbumRandomLink.vue";

vi.mock("@/api/albums", () => ({
  getRandomLink: vi.fn(),
  putRandomLink: vi.fn(),
  resetRandomLink: vi.fn(),
  deleteRandomLink: vi.fn(),
}));

const getMock = vi.mocked(getRandomLink);
const putMock = vi.mocked(putRandomLink);
const resetMock = vi.mocked(resetRandomLink);
const deleteMock = vi.mocked(deleteRandomLink);

const PATH = "/random/Ab3dE6gH9j/0123456789abcdefghijABCD";
const NEW_PATH = "/random/Ab3dE6gH9j/ZZZZZZZZZZZZZZZZZZZZZZZZ";

function link(overrides: Partial<RandomLinkView> = {}): RandomLinkView {
  return { enabled: true, path: PATH, created_at: "2026-10-07T08:00:00Z", ...overrides };
}

enableAutoUnmount(afterEach);

const writeText = vi.fn();

async function mountLink(albumId = "7") {
  const id = ref(albumId);
  const wrapper = mount(
    () => h(NMessageProvider, () => h(AlbumRandomLink, { albumId: id.value })),
    { attachTo: document.body },
  );
  await flushPromises();
  return { wrapper, id };
}

function button(wrapper: Awaited<ReturnType<typeof mountLink>>["wrapper"], text: string) {
  const found = wrapper.findAll("button").find((node) => node.text() === text);
  if (!found) throw new Error(`找不到按钮：${text}`);
  return found;
}

/** Popconfirm 面板 teleport 到 body；按文案取最后一个（最新弹出的）按钮。 */
async function confirmInBody(text: string): Promise<void> {
  const candidates = [...document.body.querySelectorAll<HTMLButtonElement>(".n-popconfirm__panel button")];
  const target = candidates.filter((node) => node.textContent?.trim() === text).at(-1);
  if (!target) throw new Error(`Popconfirm 中找不到按钮：${text}`);
  target.click();
  await flushPromises();
}

function shownUrl(wrapper: Awaited<ReturnType<typeof mountLink>>["wrapper"]): string {
  return wrapper.find<HTMLInputElement>("input[readonly]").element.value;
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
  writeText.mockResolvedValue(undefined);
  vi.stubGlobal("navigator", { userAgent: "vitest", clipboard: { writeText } });
  getMock.mockResolvedValue(null);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.resetAllMocks();
  document.body.innerHTML = "";
});

describe("AlbumRandomLink", () => {
  it("没有链接时只显示启用入口，不显示链接地址", async () => {
    const { wrapper } = await mountLink();

    expect(getMock).toHaveBeenCalledWith("7");
    expect(wrapper.find("input[readonly]").exists()).toBe(false);
    expect(button(wrapper, "启用随机链接").exists()).toBe(true);
  });

  it("首次启用先提示私有图片也会被匿名访问，确认后才创建并显示完整地址", async () => {
    putMock.mockResolvedValue(link());
    const { wrapper } = await mountLink();

    await button(wrapper, "启用随机链接").trigger("click");
    expect(putMock).not.toHaveBeenCalled();
    expect(document.body.querySelector(".n-popconfirm__panel")?.textContent).toContain(
      "私有图片",
    );

    await confirmInBody("确认启用");

    expect(putMock).toHaveBeenCalledWith("7", true);
    expect(shownUrl(wrapper)).toBe(window.location.origin + PATH);
  });

  it("首次启用取消后不发请求", async () => {
    const { wrapper } = await mountLink();

    await button(wrapper, "启用随机链接").trigger("click");
    await confirmInBody("取消");

    expect(putMock).not.toHaveBeenCalled();
    expect(wrapper.find("input[readonly]").exists()).toBe(false);
  });

  it("已有链接时显示完整地址与原图参数说明", async () => {
    getMock.mockResolvedValue(link());
    const { wrapper } = await mountLink();

    expect(shownUrl(wrapper)).toBe(window.location.origin + PATH);
    expect(wrapper.text()).toContain("?format=original");
  });

  it("已有链接切换开关直接提交，不再弹隐私提示", async () => {
    getMock.mockResolvedValue(link());
    putMock.mockResolvedValue(link({ enabled: false }));
    const { wrapper } = await mountLink();

    await wrapper.find(".n-switch").trigger("click");
    await flushPromises();

    expect(putMock).toHaveBeenCalledWith("7", false);
    expect(document.body.querySelector(".n-popconfirm__panel")).toBeNull();
    expect(wrapper.find(".n-switch").attributes("aria-checked")).toBe("false");
    expect(wrapper.text()).toContain("已停用");
  });

  it("复制按钮把完整地址写入剪贴板", async () => {
    getMock.mockResolvedValue(link());
    const { wrapper } = await mountLink();

    await button(wrapper, "复制链接").trigger("click");
    await flushPromises();

    expect(writeText).toHaveBeenCalledWith(window.location.origin + PATH);
  });

  it("重置需确认（提示旧链接失效），确认后显示新地址", async () => {
    getMock.mockResolvedValue(link());
    resetMock.mockResolvedValue(link({ path: NEW_PATH }));
    const { wrapper } = await mountLink();

    await button(wrapper, "重置链接").trigger("click");
    expect(resetMock).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("旧链接会立即失效");

    await confirmInBody("确认重置");

    expect(resetMock).toHaveBeenCalledWith("7");
    expect(shownUrl(wrapper)).toBe(window.location.origin + NEW_PATH);
  });

  it("重置取消后不发请求", async () => {
    getMock.mockResolvedValue(link());
    const { wrapper } = await mountLink();

    await button(wrapper, "重置链接").trigger("click");
    await confirmInBody("取消");

    expect(resetMock).not.toHaveBeenCalled();
    expect(shownUrl(wrapper)).toBe(window.location.origin + PATH);
  });

  it("删除确认后回到无链接状态", async () => {
    getMock.mockResolvedValue(link());
    deleteMock.mockResolvedValue(null);
    const { wrapper } = await mountLink();

    await button(wrapper, "删除链接").trigger("click");
    expect(deleteMock).not.toHaveBeenCalled();
    await confirmInBody("确认删除");

    expect(deleteMock).toHaveBeenCalledWith("7");
    expect(wrapper.find("input[readonly]").exists()).toBe(false);
    expect(button(wrapper, "启用随机链接").exists()).toBe(true);
  });

  it("保存失败时提示错误并保持操作前的状态", async () => {
    getMock.mockResolvedValue(link());
    putMock.mockRejectedValue(new Error("boom"));
    const { wrapper } = await mountLink();

    await wrapper.find(".n-switch").trigger("click");
    await flushPromises();

    expect(wrapper.find(".n-switch").attributes("aria-checked")).toBe("true");
    expect(document.body.textContent).toContain("随机链接保存失败");
    expect(shownUrl(wrapper)).toBe(window.location.origin + PATH);
  });

  it("加载失败时显示错误与重试，重试成功后恢复", async () => {
    getMock.mockRejectedValueOnce(new Error("boom"));
    const { wrapper } = await mountLink();

    expect(wrapper.text()).toContain("随机链接加载失败");
    expect(wrapper.find("input[readonly]").exists()).toBe(false);

    getMock.mockResolvedValue(link());
    await button(wrapper, "重试").trigger("click");
    await flushPromises();

    expect(shownUrl(wrapper)).toBe(window.location.origin + PATH);
  });

  it("相册切换时重新加载，且不显示上一个相册迟到的结果", async () => {
    let resolveFirst: (value: RandomLinkView | null) => void = () => {};
    getMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveFirst = resolve;
        }),
    );
    getMock.mockResolvedValueOnce(null);
    const { wrapper, id } = await mountLink("7");

    id.value = "8";
    await flushPromises();
    expect(getMock).toHaveBeenLastCalledWith("8");

    resolveFirst(link());
    await flushPromises();

    expect(wrapper.find("input[readonly]").exists()).toBe(false);
  });
});
