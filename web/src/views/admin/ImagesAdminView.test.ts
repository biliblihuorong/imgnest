import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { NDialogProvider, NMessageProvider, NInputNumber, NPagination } from "naive-ui";
import { h } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  deleteAdminImage,
  listAdminImages,
  purgeAllTrash,
} from "@/api/admin";
import { makeImage } from "@/components/images/fixtures";
import ImagesAdminView from "./ImagesAdminView.vue";

vi.mock("@/api/admin", () => ({
  listAdminImages: vi.fn(),
  deleteAdminImage: vi.fn(),
  purgeAllTrash: vi.fn(),
}));

const listAdminImagesMock = vi.mocked(listAdminImages);
const deleteAdminImageMock = vi.mocked(deleteAdminImage);
const purgeAllTrashMock = vi.mocked(purgeAllTrash);

enableAutoUnmount(afterEach);

afterEach(() => {
  vi.useRealTimers();
});

// jsdom 未实现 ResizeObserver，naive-ui 布局/弹层依赖它
class ResizeObserverStub {
  observe = vi.fn();
  unobserve = vi.fn();
  disconnect = vi.fn();
}
vi.stubGlobal("ResizeObserver", ResizeObserverStub);

function mountView(): VueWrapper {
  return mount(() => h(NMessageProvider, () => h(NDialogProvider, () => h(ImagesAdminView))));
}

function findButtonByText(host: Element | Document, text: string): HTMLButtonElement | undefined {
  return [...host.querySelectorAll("button")].find(
    (button) => button.textContent?.trim() === text,
  );
}

function bodyButton(text: string): HTMLButtonElement {
  const button = findButtonByText(document.body, text);
  if (!button) {
    throw new Error(`document.body 中找不到按钮：${text}`);
  }
  return button;
}

beforeEach(() => {
  vi.resetAllMocks();
  listAdminImagesMock.mockResolvedValue({
    items: [
      makeImage({ id: 1, name: "a.png", size: 2048, user_id: 1, is_public: true }),
      makeImage({ id: 2, name: "b.svg", ext: "svg", size: 512, user_id: 7, local_thumb_url: "" }),
    ],
    total: 42,
    page: 1,
    size: 20,
  });
});

describe("ImagesAdminView", () => {
  it("挂载后按 page=1&size=20 加载并渲染表格字段", async () => {
    const wrapper = mountView();
    await flushPromises();

    expect(listAdminImagesMock).toHaveBeenCalledWith({ page: 1, size: 20 });
    expect(wrapper.findAll("tbody tr")).toHaveLength(2);
    const text = wrapper.text();
    expect(text).toContain("a.png");
    expect(text).toContain("b.svg");
    expect(text).toContain("2.0 KB");
    expect(text).toContain("SVG");
    expect(text).toContain("公开");
    expect(text).toContain("私有");
    expect(text).toMatch(/\d{4}-\d{2}-\d{2} \d{2}:\d{2}/);
    expect(wrapper.text()).toContain("共 42 张图片");
  });

  it("列表为空时显示空状态", async () => {
    listAdminImagesMock.mockResolvedValue({ items: [], total: 0, page: 1, size: 20 });
    const wrapper = mountView();
    await flushPromises();

    expect(wrapper.text()).toContain("没有符合条件的图片");
  });

  it("关键词回车触发过滤并回到第 1 页", async () => {
    const wrapper = mountView();
    await flushPromises();

    const keyword = wrapper.find("input[placeholder='按文件名/路径搜索（回车立即搜索）']");
    await keyword.setValue("cat");
    await keyword.trigger("keyup.enter");
    await flushPromises();

    expect(listAdminImagesMock).toHaveBeenLastCalledWith({ page: 1, size: 20, keyword: "cat" });
  });

  it("user_id 变更经防抖后携带过滤参数", async () => {
    vi.useFakeTimers();
    const wrapper = mountView();
    await flushPromises();
    expect(listAdminImagesMock).toHaveBeenCalledTimes(1);

    wrapper.findAllComponents(NInputNumber)[0].vm.$emit("update:value", 7);
    expect(listAdminImagesMock).toHaveBeenCalledTimes(1);

    vi.advanceTimersByTime(400);
    await flushPromises();

    expect(listAdminImagesMock).toHaveBeenLastCalledWith({ page: 1, size: 20, user_id: 7 });
  });

  it("翻页时保留当前过滤参数", async () => {
    const wrapper = mountView();
    await flushPromises();

    const keyword = wrapper.find("input[placeholder='按文件名/路径搜索（回车立即搜索）']");
    await keyword.setValue("cat");
    await keyword.trigger("keyup.enter");
    await flushPromises();

    wrapper.findComponent(NPagination).vm.$emit("update:page", 2);
    await flushPromises();

    expect(listAdminImagesMock).toHaveBeenLastCalledWith({ page: 2, size: 20, keyword: "cat" });
  });

  it("移入回收站：Popconfirm 确认后调用 DELETE 并刷新", async () => {
    deleteAdminImageMock.mockResolvedValue(null);
    const wrapper = mountView();
    await flushPromises();
    expect(listAdminImagesMock).toHaveBeenCalledTimes(1);

    const row = wrapper.findAll("tbody tr")[0];
    await findButtonByText(row.element, "移入回收站")!.click();
    await flushPromises();

    expect(deleteAdminImageMock).not.toHaveBeenCalled();
    expect(document.body.querySelector(".n-popconfirm__panel")?.textContent).toContain(
      "移入回收站，原 URL 立即 404",
    );

    await bodyButton("确认移入").click();
    await flushPromises();

    expect(deleteAdminImageMock).toHaveBeenCalledWith(1);
    expect(listAdminImagesMock).toHaveBeenCalledTimes(2);
    expect(document.body.textContent).toContain("已移入回收站");
  });

  it("清空回收站：Dialog 强确认文案，成功后展示 purged 数量", async () => {
    purgeAllTrashMock.mockResolvedValue({ purged: 42 });
    const wrapper = mountView();
    await flushPromises();

    await findButtonByText(wrapper.element, "清空回收站")!.click();
    await flushPromises();

    // 未确认前不调用
    expect(purgeAllTrashMock).not.toHaveBeenCalled();
    const dialog = document.body.querySelector(".n-dialog");
    expect(dialog?.textContent).toContain("将永久删除回收站中所有用户的图片及其存储对象");
    expect(dialog?.textContent).toContain("不可恢复");

    await bodyButton("彻底清空").click();
    await flushPromises();

    expect(purgeAllTrashMock).toHaveBeenCalledTimes(1);
    expect(document.body.textContent).toContain("共删除 42 张图片");
  });

  it("清空回收站失败时提示错误", async () => {
    purgeAllTrashMock.mockRejectedValue(new Error("网络错误"));
    const wrapper = mountView();
    await flushPromises();

    await findButtonByText(wrapper.element, "清空回收站")!.click();
    await flushPromises();

    await bodyButton("彻底清空").click();
    await flushPromises();

    expect(document.body.textContent).toContain("清空回收站失败");
  });
});
