import { i18n } from "@vben/locales";
import { nextTick } from "vue";
import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { NMessageProvider, NPagination, NSelect, NSwitch } from "naive-ui";
import { h } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as adminApi from "@/api/admin";
import type { AdminUserPage, AdminUserView, GroupView } from "@/api/admin";
import UsersView from "./UsersView.vue";

vi.mock("@/api/admin", () => ({
  listUsers: vi.fn(),
  patchUser: vi.fn(),
  listGroups: vi.fn(),
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

function makeUser(overrides: Partial<AdminUserView> = {}): AdminUserView {
  return {
    id: 1,
    username: "alice",
    email: "alice@example.com",
    role: "user",
    status: "enabled",
    group_id: 1,
    used_bytes: 1536,
    created_at: "2026-09-01T08:00:00Z",
    ...overrides,
  };
}

function makeGroup(overrides: Partial<GroupView> = {}): GroupView {
  return {
    id: 1,
    name: "默认组",
    is_default: true,
    is_guest: false,
    capacity_bytes: 1024 ** 3,
    max_file_bytes: 10 * 1024 ** 2,
    allowed_exts: ["jpg", "png"],
    upload_per_min: 30,
    default_policy_id: 0,
    policy_ids: [1],
    user_count: 2,
    ...overrides,
  };
}

function makePage(items: AdminUserView[], totalCount = items.length): AdminUserPage {
  return { items, total: totalCount, page: 1, size: 20 };
}

function mountView(): VueWrapper {
  return mount(() => h(NMessageProvider, () => h(UsersView)));
}

beforeEach(() => {
  vi.resetAllMocks();
  adminApiMock.listUsers.mockResolvedValue(
    makePage([
      makeUser(),
      makeUser({
        id: 2,
        username: "bob",
        email: "bob@example.com",
        role: "admin",
        status: "disabled",
        group_id: 2,
        used_bytes: 0,
      }),
    ]),
  );
  adminApiMock.listGroups.mockResolvedValue([
    makeGroup(),
    makeGroup({ id: 2, name: "游客组", is_default: false, is_guest: true }),
  ]);
});

describe("UsersView", () => {
  it("挂载后按 page=1 加载并渲染用户字段", async () => {
    const wrapper = await mountView();
    await flushPromises();

    expect(adminApiMock.listUsers).toHaveBeenCalledWith({ page: 1, size: 20 });
    const text = wrapper.text();
    expect(text).toContain("共 2 个用户");
    expect(text).toContain("alice");
    expect(text).toContain("alice@example.com");
    expect(text).toContain("管理员");
    expect(text).toContain("默认组");
    expect(text).toContain("游客组");
    expect(text).toContain("1.5 KB");
    expect(text).toContain("0 B");
    expect(text).toMatch(/\d{4}-\d{2}-\d{2} \d{2}:\d{2}/);
  });

  it("列表为空显示空状态", async () => {
    adminApiMock.listUsers.mockResolvedValue(makePage([]));
    const wrapper = await mountView();
    await flushPromises();

    expect(wrapper.text()).toContain("暂无用户");
  });

  it("加载失败显示告警，重试成功后恢复列表", async () => {
    adminApiMock.listUsers.mockRejectedValue(new Error("无权限"));
    const wrapper = await mountView();
    await flushPromises();

    expect(wrapper.text()).toContain("用户列表加载失败");
    expect(wrapper.text()).not.toContain("alice");

    adminApiMock.listUsers.mockResolvedValue(makePage([makeUser()]));
    const retry = wrapper.findAll("button").find((button) => button.text() === "重试");
    expect(retry).toBeTruthy();
    await retry?.trigger("click");
    await flushPromises();

    expect(wrapper.text()).toContain("alice");
  });

  it("回车触发搜索并携带 keyword 与 page=1", async () => {
    const wrapper = await mountView();
    await flushPromises();

    const search = wrapper.find("input[placeholder='搜索用户名或邮箱']");
    await search.setValue("ali");
    await search.trigger("keyup.enter");
    await flushPromises();

    expect(adminApiMock.listUsers).toHaveBeenLastCalledWith({
      page: 1,
      size: 20,
      keyword: "ali",
    });
  });

  it("输入防抖后自动携带 keyword 搜索", async () => {
    // flushPromises 内部用 setTimeout，fake timers 下必须用 advanceTimersByTimeAsync 等微任务
    vi.useFakeTimers();
    try {
      const wrapper = mountView();
      await vi.advanceTimersByTimeAsync(0); // 初始加载
      const callsAfterMount = adminApiMock.listUsers.mock.calls.length;

      await wrapper.find("input[placeholder='搜索用户名或邮箱']").setValue("bob");
      await vi.advanceTimersByTimeAsync(499);
      expect(adminApiMock.listUsers.mock.calls.length).toBe(callsAfterMount);

      await vi.advanceTimersByTimeAsync(1);
      expect(adminApiMock.listUsers).toHaveBeenLastCalledWith({
        page: 1,
        size: 20,
        keyword: "bob",
      });
    } finally {
      vi.useRealTimers();
    }
  });

  it("翻页时携带新的 page 参数", async () => {
    const wrapper = await mountView();
    await flushPromises();

    wrapper.findComponent(NPagination).vm.$emit("update:page", 2);
    await flushPromises();

    expect(adminApiMock.listUsers).toHaveBeenLastCalledWith({ page: 2, size: 20 });
  });

  it("禁用失败：开关回滚并提示错误", async () => {
    adminApiMock.patchUser.mockRejectedValue(new Error("禁止操作"));
    const wrapper = await mountView();
    await flushPromises();

    const firstSwitch = wrapper.findAllComponents(NSwitch)[0];
    expect(firstSwitch.props("value")).toBe(true);
    firstSwitch.vm.$emit("update:value", false);
    await flushPromises();

    expect(adminApiMock.patchUser).toHaveBeenCalledWith(1, { status: "disabled" });
    expect(wrapper.findAllComponents(NSwitch)[0].props("value")).toBe(true);
    expect(document.body.textContent).toContain("状态修改失败");
  });

  it("禁用成功：更新行数据并提示", async () => {
    adminApiMock.patchUser.mockResolvedValue(makeUser({ status: "disabled" }));
    const wrapper = await mountView();
    await flushPromises();

    wrapper.findAllComponents(NSwitch)[0].vm.$emit("update:value", false);
    await flushPromises();

    expect(wrapper.findAllComponents(NSwitch)[0].props("value")).toBe(false);
    expect(document.body.textContent).toContain("已禁用「alice」");
  });

  it("调整用户组：调用 patchUser{group_id} 并按组名提示", async () => {
    adminApiMock.patchUser.mockResolvedValue(makeUser({ group_id: 2 }));
    const wrapper = await mountView();
    await flushPromises();

    wrapper.findAllComponents(NSelect)[0].vm.$emit("update:value", 2);
    await flushPromises();

    expect(adminApiMock.patchUser).toHaveBeenCalledWith(1, { group_id: 2 });
    expect(document.body.textContent).toContain("已将「alice」移至「游客组」");
  });

  it("调整用户组失败：行数据不变并提示错误", async () => {
    adminApiMock.patchUser.mockRejectedValue(new Error("没有权限"));
    const wrapper = await mountView();
    await flushPromises();

    wrapper.findAllComponents(NSelect)[0].vm.$emit("update:value", 2);
    await flushPromises();

    expect(wrapper.findAllComponents(NSelect)[0].props("value")).toBe(1);
    expect(document.body.textContent).toContain("用户组调整失败");
  });
  it("同一用户的改组与状态请求互斥，其他用户仍可操作", async () => {
    let finish!: (value: AdminUserView) => void;
    adminApiMock.patchUser.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const wrapper = mountView();
    await flushPromises();
    const selects = wrapper.findAllComponents(NSelect);
    const switches = wrapper.findAllComponents(NSwitch);
    selects[0].vm.$emit("update:value", 2);
    selects[0].vm.$emit("update:value", 2);
    switches[0].vm.$emit("update:value", false);
    await flushPromises();

    expect(adminApiMock.patchUser).toHaveBeenCalledTimes(1);
    expect(selects[0].props("disabled")).toBe(true);
    expect(switches[0].props("disabled")).toBe(true);
    expect(selects[1].props("disabled")).toBe(false);
    finish(makeUser({ group_id: 2 }));
    await flushPromises();
    expect(selects[0].props("disabled")).toBe(false);
    expect(selects[0].props("value")).toBe(2);
  });
});

it("switches visible user columns and status controls without refetching", async () => {
  const wrapper = await mountView();
  await flushPromises();
  i18n.global.locale.value = "en-US";
  await nextTick();
  expect(wrapper.text()).toContain("Users");
  expect(wrapper.text()).toContain("Email");
  expect(wrapper.find("input").attributes("placeholder")).toBe("Search username or email");
  expect(wrapper.text()).not.toContain("注册时间");
});

it("keeps the newest user search when an older response arrives later", async () => {
  let finishOld!: (value: AdminUserPage) => void;
  adminApiMock.listUsers.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finishOld = resolve;
      }),
  );
  const wrapper = await mountView();
  await flushPromises();
  const search = wrapper.find("input[placeholder='搜索用户名或邮箱']");
  adminApiMock.listUsers.mockResolvedValueOnce(makePage([makeUser({ username: "new-result" })]));
  await search.setValue("new-result");
  await search.trigger("keyup.enter");
  await flushPromises();
  finishOld(makePage([makeUser({ username: "stale-result" })]));
  await flushPromises();
  expect(wrapper.text()).toContain("new-result");
  expect(wrapper.text()).not.toContain("stale-result");
});
