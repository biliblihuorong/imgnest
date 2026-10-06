import { i18n } from "@vben/locales";
import { nextTick } from "vue";
import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { NInput, NInputNumber, NMessageProvider, NModal, NSelect } from "naive-ui";
import { h } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as adminApi from "@/api/admin";
import type { GroupView, PolicyView } from "@/api/admin";
import { ApiError } from "@/api/client";
import GroupsView from "./GroupsView.vue";

vi.mock("@/api/admin", () => ({
  listGroups: vi.fn(),
  createGroup: vi.fn(),
  updateGroup: vi.fn(),
  deleteGroup: vi.fn(),
  listPolicies: vi.fn(),
}));

const adminApiMock = vi.mocked(adminApi);

// jsdom 未实现 ResizeObserver，naive-ui 表格/弹层依赖它
class ResizeObserverStub {
  observe = vi.fn();
  unobserve = vi.fn();
  disconnect = vi.fn();
}
vi.stubGlobal("ResizeObserver", ResizeObserverStub);

enableAutoUnmount(afterEach);

const MB = 1024 * 1024;

function makeGroup(overrides: Partial<GroupView> = {}): GroupView {
  return {
    id: 1,
    name: "默认组",
    is_default: true,
    is_guest: false,
    capacity_bytes: 100 * MB,
    max_file_bytes: 10 * MB,
    allowed_exts: ["jpg", "png"],
    upload_per_min: 30,
    default_policy_id: 1,
    policy_ids: [1],
    user_count: 3,
    ...overrides,
  };
}

function makePolicy(overrides: Partial<PolicyView> = {}): PolicyView {
  return {
    id: 1,
    name: "本地策略",
    storage_id: 1,
    enabled: true,
    path_tpl: "{yyyy}/{MM}/{dd}/{random16}",
    name_tpl: "{random16}",
    webp_mode: "both",
    webp_quality: 80,
    webp_lossless: false,
    webp_effort: 4,
    max_width: 0,
    max_height: 0,
    thumb_enabled: true,
    thumb_size: 300,
    scrub_mode: "gps",
    heif_mode: "webp_only",
    link_prefer: "webp",
    on_conflict: "rename",
    strip_meta: true,
    skip_if_larger: false,
    created_at: "2026-09-01T08:00:00Z",
    updated_at: "2026-09-01T08:00:00Z",
    ...overrides,
  };
}

function bodyButton(text: string): HTMLButtonElement | null {
  return (
    Array.from(document.body.querySelectorAll("button")).find((button) =>
      button.textContent?.includes(text),
    ) ?? null
  );
}

async function mountView(): Promise<VueWrapper> {
  const wrapper = mount(() => h(NMessageProvider, () => h(GroupsView)));
  await flushPromises();
  return wrapper;
}

/** 按 placeholder 找组件（NInput/NSelect 都会把 placeholder 透传到自身 props）。 */
function findByPlaceholder<T>(list: T[], placeholder: unknown): T | undefined {
  return list.find(
    (item) => (item as { props(key: string): unknown }).props("placeholder") === placeholder,
  );
}

beforeEach(() => {
  vi.resetAllMocks();
  adminApiMock.listGroups.mockResolvedValue([
    makeGroup(),
    makeGroup({
      id: 2,
      name: "游客组",
      is_default: false,
      is_guest: true,
      capacity_bytes: 0,
      max_file_bytes: 0,
      upload_per_min: 0,
      default_policy_id: 0,
      policy_ids: [],
      user_count: 0,
    }),
  ]);
  adminApiMock.listPolicies.mockResolvedValue([
    makePolicy(),
    makePolicy({ id: 2, name: "S3 策略", storage_id: 2 }),
  ]);
});

describe("GroupsView", () => {
  it("列表渲染：0 显示「不限」，非 0 显示容量与频率", async () => {
    const wrapper = await mountView();
    const text = wrapper.text();

    expect(text).toContain("共 2 个用户组");
    expect(text).toContain("默认组");
    expect(text).toContain("游客组");
    expect(text).toContain("100 MB");
    expect(text).toContain("10.0 MB");
    // 游客组：容量/单文件/上传频率 三个 0 → 「不限」
    expect(text.split("不限").length - 1).toBeGreaterThanOrEqual(3);
    expect(text).toContain("30 次/分");
    // 默认策略列显示策略名；0 显示「—」
    expect(text).toContain("本地策略");
    expect(text).toContain("—");
  });

  it("点击新建打开表单，提交体完成 MB→字节转换", async () => {
    adminApiMock.createGroup.mockResolvedValue(makeGroup({ id: 3, name: "运维组" }));
    const wrapper = await mountView();

    const newButton = wrapper.findAll("button").find((button) => button.text() === "新建用户组");
    expect(newButton).toBeTruthy();
    await newButton?.trigger("click");
    await flushPromises();
    expect(document.body.textContent).toContain("新建用户组");

    const nameInput = findByPlaceholder(wrapper.findAllComponents(NInput), "用户组名称");
    nameInput?.vm.$emit("update:value", "运维组");

    const numbers = wrapper.findAllComponents(NInputNumber);
    expect(numbers.length).toBe(3);
    numbers[0].vm.$emit("update:value", 100); // 容量
    numbers[1].vm.$emit("update:value", 5); // 单文件上限

    const defaultSelect = findByPlaceholder(wrapper.findAllComponents(NSelect), "默认存储策略");
    defaultSelect?.vm.$emit("update:value", 2);
    const policyIdsSelect = findByPlaceholder(
      wrapper.findAllComponents(NSelect),
      "可用策略（可多选）",
    );
    policyIdsSelect?.vm.$emit("update:value", [1, 2]);

    const saveButton = bodyButton("保存");
    expect(saveButton).toBeTruthy();
    saveButton?.click();
    await flushPromises();

    expect(adminApiMock.createGroup).toHaveBeenCalledTimes(1);
    expect(adminApiMock.createGroup).toHaveBeenCalledWith({
      name: "运维组",
      capacity_bytes: 100 * MB,
      max_file_bytes: 5 * MB,
      allowed_exts: [],
      upload_per_min: 0,
      default_policy_id: 2,
      policy_ids: [1, 2],
      is_default: false,
      is_guest: false,
    });
    expect(adminApiMock.updateGroup).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("用户组「运维组」已创建");
    // 成功后关闭弹窗并刷新列表
    expect(adminApiMock.listGroups).toHaveBeenCalledTimes(2);
  });

  it("名称为空时提交被拦截，不发起请求", async () => {
    const wrapper = await mountView();

    const newButton = wrapper.findAll("button").find((button) => button.text() === "新建用户组");
    await newButton?.trigger("click");
    await flushPromises();

    const defaultSelect = findByPlaceholder(wrapper.findAllComponents(NSelect), "默认存储策略");
    defaultSelect?.vm.$emit("update:value", 1);

    bodyButton("保存")?.click();
    await flushPromises();

    expect(adminApiMock.createGroup).not.toHaveBeenCalled();
    // 表单在 NModal 内（teleport 到 body）
    expect(document.body.textContent).toContain("请输入用户组名称");
  });

  it("编辑回填：名称与 MB 数值来自 GroupView", async () => {
    const wrapper = await mountView();

    const editButton = wrapper.findAll("button").find((button) => button.text() === "编辑");
    expect(editButton).toBeTruthy();
    await editButton?.trigger("click");
    await flushPromises();

    expect(document.body.textContent).toContain("编辑用户组：默认组");
    // NInput 的根元素是 div，原生 input 在其内部
    const nameInput = findByPlaceholder(wrapper.findAllComponents(NInput), "用户组名称");
    const nameEl = nameInput?.find("input").element as HTMLInputElement | undefined;
    expect(nameEl?.value).toBe("默认组");

    const numbers = wrapper.findAllComponents(NInputNumber);
    expect(numbers[0].props("value")).toBe(100);
    expect(numbers[1].props("value")).toBe(10);
  });

  it("删除成功：确认后调用 DELETE 并刷新列表", async () => {
    adminApiMock.deleteGroup.mockResolvedValue(null);
    const wrapper = await mountView();

    const deleteButton = wrapper.findAll("button").find((button) => button.text() === "删除");
    expect(deleteButton).toBeTruthy();
    await deleteButton?.trigger("click");
    await flushPromises();
    // 仅弹出确认框，尚未调用 DELETE
    expect(adminApiMock.deleteGroup).not.toHaveBeenCalled();

    const confirmButton = Array.from(document.body.querySelectorAll("button")).find(
      (button) => button.textContent?.trim() === "确认删除",
    );
    expect(confirmButton).toBeTruthy();
    confirmButton!.click();
    await flushPromises();

    expect(adminApiMock.deleteGroup).toHaveBeenCalledWith(1);
    expect(document.body.textContent).toContain("用户组「默认组」已删除");
    expect(adminApiMock.listGroups).toHaveBeenCalledTimes(2);
  });

  it("删除返回 30008 时提示「用户组仍有成员，无法删除」且不刷新", async () => {
    adminApiMock.deleteGroup.mockRejectedValue(new ApiError(30008, "group has members", 409));
    const wrapper = await mountView();

    const deleteButton = wrapper.findAll("button").find((button) => button.text() === "删除");
    await deleteButton?.trigger("click");
    await flushPromises();

    const confirmButton = Array.from(document.body.querySelectorAll("button")).find(
      (button) => button.textContent?.trim() === "确认删除",
    );
    confirmButton!.click();
    await flushPromises();

    expect(adminApiMock.deleteGroup).toHaveBeenCalledWith(1);
    expect(document.body.textContent).toContain("用户组仍有成员，无法删除");
    expect(adminApiMock.listGroups).toHaveBeenCalledTimes(1);
  });

  it("删除其他错误时提示通用文案", async () => {
    adminApiMock.deleteGroup.mockRejectedValue(new Error("网络错误"));
    const wrapper = await mountView();

    const deleteButton = wrapper.findAll("button").find((button) => button.text() === "删除");
    await deleteButton?.trigger("click");
    await flushPromises();

    const confirmButton = Array.from(document.body.querySelectorAll("button")).find(
      (button) => button.textContent?.trim() === "确认删除",
    );
    confirmButton!.click();
    await flushPromises();

    expect(document.body.textContent).toContain("删除失败");
  });
  it("保存用户组时禁止关闭弹窗和重复提交，失败后可以取消", async () => {
    let rejectSave!: (reason: Error) => void;
    adminApiMock.updateGroup.mockImplementationOnce(
      () =>
        new Promise((_, reject) => {
          rejectSave = reject;
        }),
    );
    const wrapper = await mountView();
    await wrapper
      .findAll("button")
      .find((button) => button.text() === "编辑")!
      .trigger("click");
    await flushPromises();
    bodyButton("保存")!.click();
    bodyButton("保存")!.click();
    await flushPromises();

    expect(adminApiMock.updateGroup).toHaveBeenCalledTimes(1);
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
    bodyButton("取消")!.click();
    await flushPromises();
    expect(modal.props("show")).toBe(false);
  });
});

it("updates unlimited formats and an open form when language changes", async () => {
  const wrapper = await mountView();
  await flushPromises();
  await wrapper
    .findAll("button")
    .find((button) => button.text() === "新建用户组")!
    .trigger("click");
  await flushPromises();
  i18n.global.locale.value = "en-US";
  await nextTick();
  expect(wrapper.text()).toContain("User groups");
  expect(wrapper.text()).toContain("Unlimited");
  expect(document.body.textContent).toContain("Capacity (MB)");
  expect(document.body.textContent).toContain("Default policy");
  expect(document.body.textContent).not.toContain("允许扩展名");
});
