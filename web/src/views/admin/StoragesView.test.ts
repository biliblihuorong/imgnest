import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { NMessageProvider, NSelect, NSwitch } from "naive-ui";
import { h } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  createStorage,
  deleteStorage,
  listStorages,
  testStorage,
  updateStorage,
  type StorageView,
} from "@/api/admin";
import { ApiError } from "@/api/client";
import StoragesView from "./StoragesView.vue";

vi.mock("@/api/admin", () => ({
  listStorages: vi.fn(),
  createStorage: vi.fn(),
  updateStorage: vi.fn(),
  deleteStorage: vi.fn(),
  testStorage: vi.fn(),
}));

const listStoragesMock = vi.mocked(listStorages);
const createStorageMock = vi.mocked(createStorage);
const updateStorageMock = vi.mocked(updateStorage);
const deleteStorageMock = vi.mocked(deleteStorage);
const testStorageMock = vi.mocked(testStorage);

enableAutoUnmount(afterEach);

// jsdom 未实现 ResizeObserver，naive-ui 布局/弹层依赖它
class ResizeObserverStub {
  observe = vi.fn();
  unobserve = vi.fn();
  disconnect = vi.fn();
}
vi.stubGlobal("ResizeObserver", ResizeObserverStub);

const localFixture: StorageView = {
  id: 1,
  name: "local-main",
  driver: "local",
  base_url: "http://localhost:8080/i/1",
  enabled: true,
};

const s3Fixture: StorageView = {
  id: 2,
  name: "b2",
  driver: "s3",
  base_url: "https://cdn.example.com",
  enabled: false,
};

function mountView(): VueWrapper {
  return mount(() => h(NMessageProvider, () => h(StoragesView)));
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

/** NModal 内容 teleport 到 body，只能用原生 DOM 事件写入输入框。 */
async function setNativeInput(selector: string, value: string): Promise<void> {
  const input = document.body.querySelector<HTMLInputElement>(selector);
  if (!input) {
    throw new Error(`document.body 中找不到输入框：${selector}`);
  }
  input.value = value;
  input.dispatchEvent(new Event("input", { bubbles: true }));
  await flushPromises();
}

async function openCreateModal(wrapper: VueWrapper): Promise<void> {
  await findButtonByText(wrapper.element, "添加存储")!.click();
  await flushPromises();
}

beforeEach(() => {
  vi.resetAllMocks();
  listStoragesMock.mockResolvedValue([localFixture, s3Fixture]);
});

describe("StoragesView", () => {
  it("挂载后加载列表并渲染名称/驱动/Base URL", async () => {
    const wrapper = mountView();
    await flushPromises();

    expect(listStoragesMock).toHaveBeenCalledTimes(1);
    expect(wrapper.findAll("tbody tr")).toHaveLength(2);
    const text = wrapper.text();
    expect(text).toContain("local-main");
    expect(text).toContain("b2");
    expect(text).toContain("http://localhost:8080/i/1");
    expect(text).toContain("https://cdn.example.com");
    expect(text).toContain("密钥创建后不可查看");
  });

  it("列表加载失败时显示错误与重试", async () => {
    listStoragesMock.mockRejectedValue(new ApiError(-1, "网络错误", 0));
    const wrapper = mountView();
    await flushPromises();

    expect(wrapper.text()).toContain("存储列表加载失败");
  });

  it("启用开关失败时行数据不变（开关回滚）并提示错误", async () => {
    updateStorageMock.mockRejectedValue(new ApiError(-1, "网络错误", 0));
    const wrapper = mountView();
    await flushPromises();

    const switches = wrapper.findAllComponents(NSwitch);
    expect(switches).toHaveLength(2);
    // naive-ui 的 on-update:value 是 prop 而非 Vue 事件，需真实点击触发
    await wrapper.findAll("tbody tr")[0].find(".n-switch").trigger("click");
    await flushPromises();

    expect(updateStorageMock).toHaveBeenCalledWith(1, { enabled: false });
    // 受控开关绑定行数据：失败后行数据仍是 true，视觉回滚
    expect(wrapper.findAllComponents(NSwitch)[0].props("value")).toBe(true);
    expect(document.body.textContent).toContain("启用状态修改失败");
    expect(listStoragesMock).toHaveBeenCalledTimes(1);
  });

  it("启用开关成功后更新行数据", async () => {
    updateStorageMock.mockResolvedValue({ ...localFixture, enabled: false });
    const wrapper = mountView();
    await flushPromises();

    await wrapper.findAll("tbody tr")[0].find(".n-switch").trigger("click");
    await flushPromises();

    expect(wrapper.findAllComponents(NSwitch)[0].props("value")).toBe(false);
  });

  it("测试连接后弹窗展示 put/copy/delete 逐项结果与汇总", async () => {
    testStorageMock.mockResolvedValue({ ok: false, checks: { put: true, copy: true, delete: false } });
    const wrapper = mountView();
    await flushPromises();

    const row = wrapper.findAll("tbody tr")[1];
    await findButtonByText(row.element, "测试连接")!.click();
    await flushPromises();

    expect(testStorageMock).toHaveBeenCalledWith(2);
    const bodyText = document.body.textContent ?? "";
    expect(bodyText).toContain("存储「b2」（s3）");
    expect(bodyText).toContain("写入（put）");
    expect(bodyText).toContain("复制（copy）");
    expect(bodyText).toContain("删除（delete）");
    expect(bodyText).toContain("存在未通过的检查项");

    await bodyButton("关闭").click();
    await flushPromises();
    expect(document.body.textContent).not.toContain("存在未通过的检查项");
  });

  it("删除被规则引用（30009）时提示专用文案", async () => {
    deleteStorageMock.mockRejectedValue(new ApiError(30009, "存储仍被规则引用", 409));
    const wrapper = mountView();
    await flushPromises();

    const row = wrapper.findAll("tbody tr")[0];
    await findButtonByText(row.element, "删除")!.click();
    await flushPromises();

    expect(document.body.querySelector(".n-popconfirm__panel")?.textContent).toContain(
      "确定删除",
    );
    await bodyButton("确认删除").click();
    await flushPromises();

    expect(deleteStorageMock).toHaveBeenCalledWith(1);
    expect(document.body.textContent).toContain("该存储仍被上传规则引用，无法删除");
  });

  it("local 存储创建：提交体不含 config", async () => {
    createStorageMock.mockResolvedValue(localFixture);
    const wrapper = mountView();
    await flushPromises();

    await openCreateModal(wrapper);
    await setNativeInput("input[placeholder='例如：main']", "local-main");
    await setNativeInput("input[placeholder='例如：https://cdn.example.com']", "http://x");
    await bodyButton("创建").click();
    await flushPromises();

    expect(createStorageMock).toHaveBeenCalledWith({
      name: "local-main",
      driver: "local",
      base_url: "http://x",
    });
  });

  it("s3 存储创建：提交体携带 config 凭证字段", async () => {
    createStorageMock.mockResolvedValue(s3Fixture);
    const wrapper = mountView();
    await flushPromises();

    await openCreateModal(wrapper);
    // 驱动选择是表单里唯一的 NSelect
    wrapper.findComponent(NSelect).vm.$emit("update:value", "s3");
    await flushPromises();

    await setNativeInput("input[placeholder='例如：main']", "b2");
    await setNativeInput("input[placeholder='例如：https://cdn.example.com']", "https://cdn.example.com");
    await setNativeInput(
      "input[placeholder='例如：https://s3.us-east-1.amazonaws.com']",
      "https://s3.example.com",
    );
    await setNativeInput("input[placeholder='例如：imgnest']", "imgnest");
    await setNativeInput("input[placeholder='Access Key ID']", "AKID-1");
    await setNativeInput("input[placeholder='Secret Access Key']", "shhh-secret");

    await bodyButton("创建").click();
    await flushPromises();

    expect(createStorageMock).toHaveBeenCalledTimes(1);
    const call = createStorageMock.mock.calls[0][0];
    expect(call.name).toBe("b2");
    expect(call.driver).toBe("s3");
    expect(call.base_url).toBe("https://cdn.example.com");
    expect(call.config).toEqual({
      endpoint: "https://s3.example.com",
      region: "",
      bucket: "imgnest",
      access_key_id: "AKID-1",
      secret_access_key: "shhh-secret",
      use_path_style: false,
    });
  });

  it("编辑只提交 name/base_url，不携带 config/enabled", async () => {
    updateStorageMock.mockResolvedValue(s3Fixture);
    const wrapper = mountView();
    await flushPromises();

    const row = wrapper.findAll("tbody tr")[1];
    await findButtonByText(row.element, "编辑")!.click();
    await flushPromises();

    // 名称/地址回填；驱动被禁用；出现「不可查看」提示
    const nameInput = document.body.querySelector<HTMLInputElement>("input[placeholder='例如：main']");
    expect(nameInput?.value).toBe("b2");
    expect(document.body.textContent).toContain("密钥创建后不可查看、也不在此回显");

    await setNativeInput("input[placeholder='例如：main']", "b2-renamed");
    await bodyButton("保存").click();
    await flushPromises();

    expect(updateStorageMock).toHaveBeenCalledWith(2, {
      name: "b2-renamed",
      base_url: "https://cdn.example.com",
    });
    expect(createStorageMock).not.toHaveBeenCalled();
  });
});
