import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { NInput, NPagination } from "naive-ui";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { listImages, type ImagePage } from "@/api/images";
import { makeImage } from "@/components/images/fixtures";
import AlbumCoverPicker from "./AlbumCoverPicker.vue";

vi.mock("@/api/images", () => ({ listImages: vi.fn() }));
vi.mock("@/api/thumbnails", () => ({
  fetchProtectedThumbnail: vi.fn().mockResolvedValue(new Blob()),
}));
const listMock = vi.mocked(listImages);
enableAutoUnmount(afterEach);

function page(name: string, current = 1): ImagePage {
  return { items: [makeImage({ name, id: current })], page: current, size: 24, total: 72 };
}

function mountPicker() {
  return mount(AlbumCoverPicker, {
    props: { selected: 0 },
    global: { stubs: { teleport: true } },
  });
}

beforeEach(() => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  URL.createObjectURL = vi.fn(() => "blob:thumbnail");
  URL.revokeObjectURL = vi.fn();
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  listMock.mockReset();
});

describe("AlbumCoverPicker pending requests", () => {
  it("加载期间搜索，最新关键词结果优先，迟到旧结果不覆盖", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    let finishOld!: (value: ImagePage) => void;
    listMock.mockReturnValueOnce(
      new Promise((resolve) => {
        finishOld = resolve;
      }),
    );
    listMock.mockResolvedValueOnce(page("new-search.png"));
    const wrapper = mountPicker();
    wrapper.findComponent(NInput).vm.$emit("update:value", "new-search");
    await wrapper.vm.$nextTick();
    await vi.advanceTimersByTimeAsync(350);
    await flushPromises();
    expect(wrapper.find("[aria-label='将 new-search.png 设为相册封面']").exists()).toBe(true);
    finishOld(page("old.png"));
    await flushPromises();
    expect(wrapper.text()).not.toContain("old.png");
    expect(listMock).toHaveBeenLastCalledWith(
      { page: 1, size: 24, q: "new-search" },
      expect.any(AbortSignal),
    );
  });

  it("加载期间继续翻页，保持最新页的结果且忽略旧请求失败", async () => {
    let rejectOld!: (cause: Error) => void;
    listMock.mockResolvedValueOnce(page("page-1.png"));
    const wrapper = mountPicker();
    await flushPromises();
    listMock.mockReturnValueOnce(
      new Promise((_, reject) => {
        rejectOld = reject;
      }),
    );
    wrapper.findComponent(NPagination).vm.$emit("update:page", 2);
    await wrapper.vm.$nextTick();
    listMock.mockResolvedValueOnce(page("page-3.png", 3));
    wrapper.findComponent(NPagination).vm.$emit("update:page", 3);
    await flushPromises();
    expect(wrapper.find("[aria-label='将 page-3.png 设为相册封面']").exists()).toBe(true);
    rejectOld(new Error("old request failed"));
    await flushPromises();
    expect(wrapper.text()).not.toContain("old request failed");
    expect(wrapper.findComponent(NPagination).props("page")).toBe(3);
    expect(listMock).toHaveBeenLastCalledWith(
      { page: 3, size: 24, q: undefined },
      expect.any(AbortSignal),
    );
  });
  it("替换列表条件或销毁选择器时撤销在途请求", async () => {
    listMock.mockResolvedValueOnce(page("page-1.png"));
    const wrapper = mountPicker();
    await flushPromises();
    listMock.mockReturnValue(new Promise(() => {}));
    wrapper.findComponent(NPagination).vm.$emit("update:page", 2);
    await wrapper.vm.$nextTick();
    const second = listMock.mock.calls[1]?.[1];
    expect(second).toBeInstanceOf(AbortSignal);
    wrapper.findComponent(NPagination).vm.$emit("update:page", 3);
    await wrapper.vm.$nextTick();
    expect(second?.aborted).toBe(true);
    const third = listMock.mock.calls[2]?.[1];
    wrapper.unmount();
    expect(third?.aborted).toBe(true);
  });
});
