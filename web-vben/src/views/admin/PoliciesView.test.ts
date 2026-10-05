import { i18n } from "@vben/locales";
import { nextTick } from "vue";
import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { NMessageProvider, NModal, NSelect } from "naive-ui";
import { h } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  createPolicy,
  deletePolicy,
  listPolicies,
  listStorages,
  previewPolicy,
  updatePolicy,
  type PolicyView,
  type StorageView,
} from "@/api/admin";
import PoliciesView from "./PoliciesView.vue";

vi.mock("@/api/admin", () => ({
  listPolicies: vi.fn(),
  listStorages: vi.fn(),
  createPolicy: vi.fn(),
  updatePolicy: vi.fn(),
  deletePolicy: vi.fn(),
  previewPolicy: vi.fn(),
}));

const listPoliciesMock = vi.mocked(listPolicies);
const listStoragesMock = vi.mocked(listStorages);
const createPolicyMock = vi.mocked(createPolicy);
const updatePolicyMock = vi.mocked(updatePolicy);
const deletePolicyMock = vi.mocked(deletePolicy);
const previewPolicyMock = vi.mocked(previewPolicy);

enableAutoUnmount(afterEach);

// jsdom 未实现 ResizeObserver，naive-ui 布局/弹层依赖它
class ResizeObserverStub {
  observe = vi.fn();
  unobserve = vi.fn();
  disconnect = vi.fn();
}
vi.stubGlobal("ResizeObserver", ResizeObserverStub);

const storageFixture: StorageView = {
  id: 2,
  name: "main",
  driver: "local",
  base_url: "http://localhost:9000/imgnest",
  enabled: true,
};

const policyFixture: PolicyView = {
  id: 1,
  name: "默认规则",
  storage_id: 2,
  enabled: true,
  path_tpl: "{Y}/{m}/{d}",
  name_tpl: "{uniqid}",
  webp_mode: "both",
  webp_quality: 80,
  webp_lossless: false,
  webp_effort: 4,
  max_width: 0,
  max_height: 0,
  thumb_enabled: true,
  thumb_size: 400,
  scrub_mode: "gps",
  heif_mode: "webp_only",
  link_prefer: "webp",
  on_conflict: "rename",
  strip_meta: true,
  skip_if_larger: true,
  created_at: "2026-10-04T00:00:00Z",
  updated_at: "2026-10-04T00:00:00Z",
};

function mountView(): VueWrapper {
  return mount(() => h(NMessageProvider, () => h(PoliciesView)));
}

function findButtonByText(host: Element | Document, text: string): HTMLButtonElement | undefined {
  return [...host.querySelectorAll("button")].find((button) => button.textContent?.trim() === text);
}

function bodyButton(text: string): HTMLButtonElement {
  const button = findButtonByText(document.body, text);
  if (!button) {
    throw new Error(`document.body 中找不到按钮：${text}`);
  }
  return button;
}

async function setNativeInput(selector: string, value: string): Promise<void> {
  const input = document.body.querySelector<HTMLInputElement>(selector);
  if (!input) {
    throw new Error(`document.body 中找不到输入框：${selector}`);
  }
  input.value = value;
  input.dispatchEvent(new Event("input", { bubbles: true }));
  await flushPromises();
}

/** 表单里带 placeholder 的 NSelect（存储选择器）。 */
function storageSelect(wrapper: VueWrapper): VueWrapper {
  const select = wrapper
    .findAllComponents(NSelect)
    .find((item) => item.props("placeholder") === "选择存储");
  if (!select) {
    throw new Error("找不到存储选择器");
  }
  return select;
}

async function openCreateModal(wrapper: VueWrapper): Promise<void> {
  await findButtonByText(wrapper.element, "新建规则")!.click();
  await flushPromises();
}

beforeEach(() => {
  vi.resetAllMocks();
  listPoliciesMock.mockResolvedValue([policyFixture]);
  listStoragesMock.mockResolvedValue([storageFixture]);
});

describe("PoliciesView", () => {
  it("挂载后加载规则与存储并渲染列表字段", async () => {
    const wrapper = mountView();
    await flushPromises();

    expect(listPoliciesMock).toHaveBeenCalledTimes(1);
    expect(listStoragesMock).toHaveBeenCalledTimes(1);
    expect(wrapper.findAll("tbody tr")).toHaveLength(1);
    const text = wrapper.text();
    expect(text).toContain("默认规则");
    expect(text).toContain("main");
    expect(text).toContain("{Y}/{m}/{d}");
    expect(text).toContain("{uniqid}");
    expect(text).toContain("原图+WebP");
    expect(text).toContain("抹除 GPS");
    expect(text).toContain("启用");
  });

  it("列表加载失败时显示错误与重试", async () => {
    listPoliciesMock.mockRejectedValue(new Error("网络错误"));
    const wrapper = mountView();
    await flushPromises();

    expect(wrapper.text()).toContain("规则列表加载失败");
  });

  it("新建：提交体含 name/storage_id 与模板默认值", async () => {
    createPolicyMock.mockResolvedValue(policyFixture);
    const wrapper = mountView();
    await flushPromises();

    await openCreateModal(wrapper);
    await setNativeInput("input[placeholder='例如：默认规则']", "新规则");
    storageSelect(wrapper).vm.$emit("update:value", 2);
    await flushPromises();

    await bodyButton("创建").click();
    await flushPromises();

    expect(createPolicyMock).toHaveBeenCalledTimes(1);
    expect(createPolicyMock).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "新规则",
        storage_id: 2,
        path_tpl: "{Y}/{m}/{d}",
        name_tpl: "{uniqid}",
        webp_mode: "both",
        webp_quality: 80,
        thumb_size: 400,
        scrub_mode: "gps",
        on_conflict: "rename",
        enabled: true,
      }),
    );
  });

  it("新建：名称为空或未选存储时不发请求", async () => {
    const wrapper = mountView();
    await flushPromises();

    await openCreateModal(wrapper);
    await bodyButton("创建").click();
    await flushPromises();

    expect(createPolicyMock).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("请输入规则名称");

    await setNativeInput("input[placeholder='例如：默认规则']", "新规则");
    await bodyButton("创建").click();
    await flushPromises();

    expect(createPolicyMock).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("请选择存储");
  });

  it("路径预览成功：展示样例路径；模板错误：展示错误描述", async () => {
    const wrapper = mountView();
    await flushPromises();

    await openCreateModal(wrapper);

    previewPolicyMock.mockResolvedValueOnce({ sample: "2026/10/04/ab12cd34.webp", error: "" });
    await bodyButton("预览路径").click();
    await flushPromises();

    expect(previewPolicyMock).toHaveBeenCalledWith({
      path_tpl: "{Y}/{m}/{d}",
      name_tpl: "{uniqid}",
    });
    expect(document.body.textContent).toContain("样例：2026/10/04/ab12cd34.webp");

    previewPolicyMock.mockResolvedValueOnce({ sample: "", error: "未知变量 {foo}" });
    await bodyButton("预览路径").click();
    await flushPromises();

    expect(document.body.textContent).toContain("模板错误：模板无效，请检查路径与文件名模板");
    expect(document.body.textContent).not.toContain("样例：");
  });

  it("路径预览请求失败时提示错误", async () => {
    previewPolicyMock.mockRejectedValue(new Error("网络错误"));
    const wrapper = mountView();
    await flushPromises();

    await openCreateModal(wrapper);
    await bodyButton("预览路径").click();
    await flushPromises();

    expect(document.body.textContent).toContain("路径预览失败");
  });

  it("编辑：回填行数据，保存时携带 id 提交", async () => {
    updatePolicyMock.mockResolvedValue(policyFixture);
    const wrapper = mountView();
    await flushPromises();

    const row = wrapper.findAll("tbody tr")[0];
    await findButtonByText(row.element, "编辑")!.click();
    await flushPromises();

    const nameInput = document.body.querySelector<HTMLInputElement>(
      "input[placeholder='例如：默认规则']",
    );
    expect(nameInput?.value).toBe("默认规则");
    expect((storageSelect(wrapper).props() as { value?: number }).value).toBe(2);

    await setNativeInput("input[placeholder='例如：默认规则']", "改名规则");
    await bodyButton("保存").click();
    await flushPromises();

    expect(updatePolicyMock).toHaveBeenCalledTimes(1);
    expect(updatePolicyMock).toHaveBeenCalledWith(
      1,
      expect.objectContaining({ name: "改名规则", storage_id: 2 }),
    );
    expect(createPolicyMock).not.toHaveBeenCalled();
  });

  it("删除：Popconfirm 确认后调用 DELETE 并刷新", async () => {
    deletePolicyMock.mockResolvedValue(null);
    const wrapper = mountView();
    await flushPromises();

    const row = wrapper.findAll("tbody tr")[0];
    await findButtonByText(row.element, "删除")!.click();
    await flushPromises();

    expect(deletePolicyMock).not.toHaveBeenCalled();
    expect(document.body.querySelector(".n-popconfirm__panel")?.textContent).toContain("确定删除");

    await bodyButton("确认删除").click();
    await flushPromises();

    expect(deletePolicyMock).toHaveBeenCalledWith(1);
    expect(listPoliciesMock).toHaveBeenCalledTimes(2);
    expect(document.body.textContent).toContain("已删除");
  });
  it("保存规则时禁止关闭弹窗和重复提交，失败后可以取消", async () => {
    let rejectSave!: (reason: Error) => void;
    updatePolicyMock.mockImplementationOnce(
      () =>
        new Promise((_, reject) => {
          rejectSave = reject;
        }),
    );
    const wrapper = mountView();
    await flushPromises();
    findButtonByText(wrapper.findAll("tbody tr")[0].element, "编辑")!.click();
    await flushPromises();
    bodyButton("保存").click();
    bodyButton("保存").click();
    await flushPromises();

    expect(updatePolicyMock).toHaveBeenCalledTimes(1);
    const modal = wrapper.findComponent(NModal);
    expect(modal.props("maskClosable")).toBe(false);
    expect(modal.props("closeOnEsc")).toBe(false);
    expect(modal.props("closable")).toBe(false);
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    document.querySelector<HTMLElement>(".n-modal-mask")?.click();
    await flushPromises();
    expect(modal.props("show")).toBe(true);
    rejectSave(new Error("保存中断"));
    await flushPromises();
    expect(modal.props("closable")).toBe(true);
    bodyButton("取消").click();
    await flushPromises();
    expect(modal.props("show")).toBe(false);
  });

  it("编辑模板后立即清除旧预览", async () => {
    previewPolicyMock.mockResolvedValue({ sample: "old/preview.webp", error: "" });
    const wrapper = mountView();
    await flushPromises();
    await openCreateModal(wrapper);
    bodyButton("预览路径").click();
    await flushPromises();
    expect(document.body.textContent).toContain("样例：old/preview.webp");
    await setNativeInput("input[placeholder='{Y}/{m}/{d}']", "new/{Y}");
    expect(document.body.textContent).not.toContain("样例：");
  });

  it("关闭后重开不显示旧预览，先前请求的迟到响应不会写入新表单", async () => {
    let finish!: (value: { sample: string; error: string }) => void;
    previewPolicyMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const wrapper = mountView();
    await flushPromises();
    await openCreateModal(wrapper);
    bodyButton("预览路径").click();
    await flushPromises();
    bodyButton("取消").click();
    await flushPromises();
    await openCreateModal(wrapper);
    finish({ sample: "old/preview.webp", error: "" });
    await flushPromises();
    expect(document.body.textContent).not.toContain("样例：");
    previewPolicyMock.mockResolvedValueOnce({ sample: "new/preview.webp", error: "" });
    bodyButton("预览路径").click();
    await flushPromises();
    expect(document.body.textContent).toContain("样例：new/preview.webp");
  });
});

it("keeps an open rule form and draft when switching languages", async () => {
  const wrapper = await mountView();
  await flushPromises();
  await wrapper
    .findAll("button")
    .find((button) => button.text() === "新建规则")!
    .trigger("click");
  await flushPromises();
  await setNativeInput("input[placeholder='例如：默认规则']", "My draft");
  i18n.global.locale.value = "en-US";
  await nextTick();
  expect(wrapper.text()).toContain("Upload policies");
  expect(document.body.textContent).toContain("Transcoding");
  const input = document.body.querySelector<HTMLInputElement>(
    "input[placeholder='e.g. Default policy']",
  );
  expect(input?.value).toBe("My draft");
  expect(document.body.textContent).not.toContain("规则名称");
});
