import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NDialogProvider, NMessageProvider, NPagination } from "naive-ui";
import { h } from "vue";
import { listTrash, purgeImages, restoreImages } from "@/api/images";
import { makeImage } from "@/components/images/fixtures";
import TrashView from "./TrashView.vue";

vi.mock("@/api/images", () => ({
  listTrash: vi.fn(),
  restoreImages: vi.fn(),
  purgeImages: vi.fn(),
}));

const listTrashMock = vi.mocked(listTrash);
const restoreImagesMock = vi.mocked(restoreImages);
const purgeImagesMock = vi.mocked(purgeImages);

enableAutoUnmount(afterEach);

function mountTrash() {
  return mount(() => h(NMessageProvider, () => h(NDialogProvider, () => h(TrashView))));
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

async function checkRow(wrapper: Awaited<ReturnType<typeof mountTrash>>, rowIndex: number) {
  // naive-ui 2.45 的 Checkbox 不渲染真实 input，根节点 div 自带 onClick
  const row = wrapper.findAll("tbody tr")[rowIndex];
  await row.find(".n-checkbox").trigger("click");
}

beforeEach(() => {
  vi.resetAllMocks();
  listTrashMock.mockResolvedValue({
    items: [
      makeImage({
        id: 1,
        name: "a.png",
        deleted_at: "2026-10-03T00:00:00Z",
        purge_at: new Date(Date.now() + 72 * 3_600_000).toISOString(),
      }),
      makeImage({
        id: 2,
        name: "b.jpg",
        deleted_at: "2026-10-03T00:00:00Z",
        purge_at: new Date(Date.now() + 6 * 3_600_000).toISOString(),
      }),
    ],
    total: 2,
    page: 1,
    size: 20,
  });
});

describe("TrashView", () => {
  it("加载回收站列表，展示名称、大小与剩余保留时间", async () => {
    const wrapper = await mountTrash();
    await flushPromises();

    expect(listTrashMock).toHaveBeenCalledWith({ page: 1, size: 20 });
    expect(wrapper.findAll("tbody tr")).toHaveLength(2);
    expect(wrapper.text()).toContain("a.png");
    expect(wrapper.text()).toContain("2.0 KB");
    expect(wrapper.text()).toContain("剩余");
  });

  it("回收站为空时显示空状态", async () => {
    listTrashMock.mockResolvedValue({ items: [], total: 0, page: 1, size: 20 });
    const wrapper = await mountTrash();
    await flushPromises();

    expect(wrapper.text()).toContain("回收站为空");
  });

  it("单行恢复：Popconfirm 确认后调用 restoreImages 并刷新", async () => {
    restoreImagesMock.mockResolvedValue([
      { id: 1, status: 200, code: 0, message: "ok", data: null },
    ]);
    const wrapper = await mountTrash();
    await flushPromises();

    const row = wrapper.findAll("tbody tr")[0];
    await findButtonByText(row.element, "恢复")!.click();
    await flushPromises();

    await bodyButton("恢复").click();
    await flushPromises();

    expect(restoreImagesMock).toHaveBeenCalledWith([1]);
    expect(listTrashMock).toHaveBeenCalledTimes(2);
    expect(document.body.textContent).toContain("恢复成功 1 张");
  });

  it("单行彻底删除：确认文案说明不可恢复，确认后调用 purgeImages", async () => {
    purgeImagesMock.mockResolvedValue([
      { id: 2, status: 200, code: 0, message: "ok", data: null },
    ]);
    const wrapper = await mountTrash();
    await flushPromises();

    const row = wrapper.findAll("tbody tr")[1];
    await findButtonByText(row.element, "彻底删除")!.click();
    await flushPromises();

    const popconfirm = document.body.querySelector(".n-popconfirm__panel");
    expect(popconfirm?.textContent).toContain("不可恢复");

    await bodyButton("彻底删除").click();
    await flushPromises();

    expect(purgeImagesMock).toHaveBeenCalledWith([2]);
    expect(document.body.textContent).toContain("彻底删除成功 1 张");
  });

  it("勾选后批量恢复：Dialog 确认后调用 restoreImages 并清空勾选", async () => {
    restoreImagesMock.mockResolvedValue([
      { id: 1, status: 200, code: 0, message: "ok", data: null },
      { id: 2, status: 200, code: 0, message: "ok", data: null },
    ]);
    const wrapper = await mountTrash();
    await flushPromises();

    await checkRow(wrapper, 0);
    await checkRow(wrapper, 1);
    expect(wrapper.findAll("tbody .n-checkbox--checked")).toHaveLength(2);

    const restoreSelected = [...wrapper.findAll("button")].find(
      (button) => button.text().trim() === "恢复选中",
    );
    expect(restoreSelected).toBeDefined();
    await restoreSelected!.trigger("click");
    await flushPromises();

    const dialog = document.body.querySelector(".n-dialog");
    expect(dialog?.textContent).toContain("将选中的 2 张图片恢复到原路径");

    await bodyButton("恢复").click();
    await flushPromises();

    expect(restoreImagesMock).toHaveBeenCalledWith([1, 2]);
    expect(listTrashMock).toHaveBeenCalledTimes(2);
    expect(document.body.textContent).toContain("恢复成功 2 张");
    expect(wrapper.findAll("tbody .n-checkbox--checked")).toHaveLength(0);
  });

  it("批量彻底删除：Dialog 使用不可恢复的强确认文案", async () => {
    purgeImagesMock.mockResolvedValue([
      { id: 1, status: 200, code: 0, message: "ok", data: null },
    ]);
    const wrapper = await mountTrash();
    await flushPromises();

    await checkRow(wrapper, 0);
    await [...wrapper.findAll("button")]
      .find((button) => button.text().trim() === "彻底删除选中")!
      .trigger("click");
    await flushPromises();

    const dialog = document.body.querySelector(".n-dialog");
    expect(dialog?.textContent).toContain("不可恢复");

    await bodyButton("彻底删除").click();
    await flushPromises();

    expect(purgeImagesMock).toHaveBeenCalledWith([1]);
  });

  it("批量恢复逐项失败时逐项反馈", async () => {
    restoreImagesMock.mockResolvedValue([
      { id: 1, status: 404, code: 30002, message: "图片不存在", data: null },
      { id: 2, status: 200, code: 0, message: "ok", data: null },
    ]);
    const wrapper = await mountTrash();
    await flushPromises();

    await checkRow(wrapper, 0);
    await checkRow(wrapper, 1);
    await [...wrapper.findAll("button")]
      .find((button) => button.text().trim() === "恢复选中")!
      .trigger("click");
    await flushPromises();

    await bodyButton("恢复").click();
    await flushPromises();

    expect(document.body.textContent).toContain("恢复成功 1 张");
    expect(document.body.textContent).toContain("恢复失败（ID 1）：图片不存在");
  });

  it("翻页时携带新的 page 参数", async () => {
    const wrapper = await mountTrash();
    await flushPromises();

    wrapper.findComponent(NPagination).vm.$emit("update:page", 2);
    await flushPromises();

    expect(listTrashMock).toHaveBeenLastCalledWith({ page: 2, size: 20 });
  });
});
