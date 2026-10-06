import { i18n } from "@vben/locales";
import { nextTick } from "vue";
import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import {
  NFormItem,
  NInput,
  NMessageProvider,
  NModal,
  NPagination,
  NSelect,
  NSwitch,
} from "naive-ui";
import { createPinia, setActivePinia } from "pinia";
import { useAuthStore } from "@/stores/auth";
import { ApiError, TOKEN_STORAGE_KEY } from "@/api/client";
import { h } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as adminApi from "@/api/admin";
import type { AdminUserPage, AdminUserView, GroupView } from "@/api/admin";
import UsersView from "./UsersView.vue";

vi.mock("@/api/admin", () => ({
  listUsers: vi.fn(),
  createUser: vi.fn(),
  patchUser: vi.fn(),
  listGroups: vi.fn(),
}));

const adminApiMock = vi.mocked(adminApi);
const { replaceRoute } = vi.hoisted(() => ({ replaceRoute: vi.fn() }));
vi.mock("vue-router", () => ({ useRouter: () => ({ replace: replaceRoute }) }));
let pinia = createPinia();

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
    display_name: "",
    avatar_provider: "weavatar" as const,
    avatar_url: null,
    avatar_config_version: 0,
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
  return mount(() => h(NMessageProvider, () => h(UsersView)), { global: { plugins: [pinia] } });
}

beforeEach(() => {
  vi.resetAllMocks();
  localStorage.clear();
  pinia = createPinia();
  setActivePinia(pinia);
  replaceRoute.mockResolvedValue(undefined);
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
    makeGroup({ id: 2, name: "专业组", is_default: false }),
    makeGroup({ id: 3, name: "游客组", is_default: false, is_guest: true }),
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
    expect(text).toContain("专业组");
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
    expect(document.body.textContent).toContain("已将「alice」移至「专业组」");
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

function bodyButton(text: string): HTMLButtonElement {
  const button = Array.from(document.body.querySelectorAll("button")).find(
    (item) => item.textContent?.trim() === text,
  );
  expect(button, `button ${text}`).toBeTruthy();
  return button!;
}

function formField(wrapper: VueWrapper, path: string) {
  const field = wrapper.findAllComponents(NFormItem).find((item) => item.props("path") === path);
  expect(field, `form field ${path}`).toBeTruthy();
  return field!;
}

async function setText(wrapper: VueWrapper, path: string, value: string) {
  formField(wrapper, path).findComponent(NInput).vm.$emit("update:value", value);
  await nextTick();
}

async function setChoice(wrapper: VueWrapper, path: string, value: string | number | null) {
  formField(wrapper, path).findComponent(NSelect).vm.$emit("update:value", value);
  await nextTick();
}

async function openCreate(wrapper: VueWrapper) {
  const button = wrapper.findAll("button").find((item) => item.text() === "新建用户");
  expect(button).toBeTruthy();
  await button!.trigger("click");
  await flushPromises();
}

async function openEdit(wrapper: VueWrapper, index = 0) {
  const button = wrapper.findAll("button").filter((item) => item.text() === "编辑")[index];
  expect(button).toBeTruthy();
  await button!.trigger("click");
  await flushPromises();
}

async function fillCreate(wrapper: VueWrapper) {
  await setText(wrapper, "username", " charlie ");
  await setText(wrapper, "email", " CHARLIE@EXAMPLE.COM ");
  await setText(wrapper, "password", "initial-password-123");
  await setText(wrapper, "display_name", " Charlie ");
}

function signInAs(user = makeUser()) {
  const auth = useAuthStore(pinia);
  auth.token = "current-session";
  auth.user = user;
  localStorage.setItem(TOKEN_STORAGE_KEY, auth.token);
  return auth;
}

describe("admin account forms", () => {
  it("creates a user with all account fields, normalized identity and initial password", async () => {
    adminApiMock.createUser.mockResolvedValue(makeUser({ id: 4, username: "charlie" }));
    const wrapper = mountView();
    await flushPromises();
    await openCreate(wrapper);
    await fillCreate(wrapper);
    await setChoice(wrapper, "role", "admin");
    await setChoice(wrapper, "status", "disabled");
    await setChoice(wrapper, "group_id", 2);
    bodyButton("创建").click();
    await flushPromises();
    expect(adminApiMock.createUser).toHaveBeenCalledExactlyOnceWith({
      username: "charlie",
      email: "charlie@example.com",
      password: "initial-password-123",
      display_name: "Charlie",
      role: "admin",
      status: "disabled",
      group_id: 2,
    });
    expect(adminApiMock.patchUser).not.toHaveBeenCalled();
    expect(wrapper.findComponent(NModal).props("show")).toBe(false);
    expect(adminApiMock.listUsers).toHaveBeenCalledTimes(2);
    expect(document.body.textContent).toContain("用户「charlie」已创建");
  });

  it("prefills edits, hides password, preserves avatar and submits only changed fields", async () => {
    adminApiMock.patchUser.mockResolvedValue(
      makeUser({
        username: "alice-new",
        email: "new@example.com",
        display_name: "New name",
        role: "admin",
        status: "disabled",
        group_id: 2,
      }),
    );
    const wrapper = mountView();
    await flushPromises();
    await openEdit(wrapper);
    expect(formField(wrapper, "username").findComponent(NInput).props("value")).toBe("alice");
    expect(formField(wrapper, "email").findComponent(NInput).props("value")).toBe(
      "alice@example.com",
    );
    expect(
      wrapper.findAllComponents(NFormItem).some((item) => item.props("path") === "password"),
    ).toBe(false);
    expect(document.body.querySelector('input[type="password"]')).toBeNull();
    await setText(wrapper, "username", " alice-new ");
    await setText(wrapper, "email", " NEW@EXAMPLE.COM ");
    await setText(wrapper, "display_name", " New name ");
    await setChoice(wrapper, "role", "admin");
    await setChoice(wrapper, "status", "disabled");
    await setChoice(wrapper, "group_id", 2);
    bodyButton("保存").click();
    await flushPromises();
    expect(adminApiMock.patchUser).toHaveBeenCalledExactlyOnceWith(1, {
      username: "alice-new",
      email: "new@example.com",
      display_name: "New name",
      role: "admin",
      status: "disabled",
      group_id: 2,
    });
    expect(adminApiMock.createUser).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("用户「alice-new」已更新");
  });

  it.each([
    ["username", "  ", "用户名须为 3–64 个字符"],
    ["username", "ab", "用户名须为 3–64 个字符"],
    ["username", "a".repeat(65), "用户名须为 3–64 个字符"],
    ["email", "not-an-email", "请输入有效的邮箱地址"],
    ["email", "Alice <alice@example.com>", "请输入有效的邮箱地址"],
    ["password", "", "初始密码须为 12–72 字节"],
    ["password", "short", "初始密码须为 12–72 字节"],
    ["password", "密".repeat(25), "初始密码须为 12–72 字节"],
    ["display_name", "😀".repeat(65), "显示名称最多 64 个字符，且不能含控制字符"],
    ["display_name", "bad\u0000name", "显示名称最多 64 个字符，且不能含控制字符"],
  ])("validates %s before sending a create request", async (path, value, error) => {
    const wrapper = mountView();
    await flushPromises();
    await openCreate(wrapper);
    await fillCreate(wrapper);
    await setText(wrapper, path, value);
    bodyButton("创建").click();
    await flushPromises();
    expect(adminApiMock.createUser).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain(error);
    expect(wrapper.findComponent(NModal).props("show")).toBe(true);
  });

  it("counts Unicode names by code points and passwords by UTF-8 bytes", async () => {
    adminApiMock.createUser.mockResolvedValue(makeUser({ id: 4 }));
    const wrapper = mountView();
    await flushPromises();
    await openCreate(wrapper);
    await fillCreate(wrapper);
    await setText(wrapper, "username", "😀".repeat(64));
    await setText(wrapper, "display_name", "😀".repeat(64));
    await setText(wrapper, "password", "密".repeat(4));
    bodyButton("创建").click();
    await flushPromises();
    expect(adminApiMock.createUser).toHaveBeenCalledOnce();
  });

  it("filters guest groups from inline and form options and blocks forged selections", async () => {
    const wrapper = mountView();
    await flushPromises();
    for (const select of wrapper.findAllComponents(NSelect)) {
      expect(select.props("options")).toEqual([
        { label: "默认组", value: 1 },
        { label: "专业组", value: 2 },
      ]);
    }
    wrapper.findAllComponents(NSelect)[0].vm.$emit("update:value", 3);
    await flushPromises();
    expect(adminApiMock.patchUser).not.toHaveBeenCalled();
    await openCreate(wrapper);
    await fillCreate(wrapper);
    expect(formField(wrapper, "group_id").findComponent(NSelect).props("value")).toBe(1);
    expect(formField(wrapper, "group_id").findComponent(NSelect).props("options")).toHaveLength(2);
    await setChoice(wrapper, "group_id", 3);
    bodyButton("创建").click();
    await flushPromises();
    expect(adminApiMock.createUser).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("请选择非游客用户组");
  });

  it.each([
    ["role", "owner"],
    ["status", "deleted"],
  ])("rejects an invalid %s selection", async (path, value) => {
    const wrapper = mountView();
    await flushPromises();
    await openCreate(wrapper);
    await fillCreate(wrapper);
    await setChoice(wrapper, path, value);
    bodyButton("创建").click();
    await flushPromises();
    expect(adminApiMock.createUser).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("请选择有效的角色和状态");
  });

  it("validates edits as well as newly created accounts", async () => {
    const wrapper = mountView();
    await flushPromises();
    await openEdit(wrapper);
    await setText(wrapper, "username", "ab");
    bodyButton("保存").click();
    await flushPromises();
    expect(adminApiMock.patchUser).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("用户名须为 3–64 个字符");
  });

  it("requires an explicitly selected group when no default is available", async () => {
    adminApiMock.listGroups.mockResolvedValue([makeGroup({ is_default: false })]);
    const wrapper = mountView();
    await flushPromises();
    await openCreate(wrapper);
    await fillCreate(wrapper);
    expect(formField(wrapper, "group_id").findComponent(NSelect).props("value")).toBeNull();
    bodyButton("创建").click();
    await flushPromises();
    expect(adminApiMock.createUser).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("请选择非游客用户组");
  });

  it.each(["create", "edit"])(
    "keeps the %s form open on API errors with localized feedback",
    async (mode) => {
      adminApiMock.createUser.mockRejectedValue(new ApiError(30002, "private diagnostic", 409));
      adminApiMock.patchUser.mockRejectedValue(new ApiError(30002, "private diagnostic", 409));
      const wrapper = mountView();
      await flushPromises();
      if (mode === "create") {
        await openCreate(wrapper);
        await fillCreate(wrapper);
      } else {
        await openEdit(wrapper);
        await setText(wrapper, "email", "duplicate@example.com");
      }
      bodyButton(mode === "create" ? "创建" : "保存").click();
      await flushPromises();
      expect(document.body.textContent).toContain("账号已存在");
      expect(document.body.textContent).not.toContain("private diagnostic");
      expect(wrapper.findComponent(NModal).props("show")).toBe(true);
      expect(adminApiMock.listUsers).toHaveBeenCalledTimes(1);
    },
  );

  it("prevents repeated submission and dismissal while saving, then permits retry or cancellation", async () => {
    let rejectSave!: (error: Error) => void;
    adminApiMock.createUser.mockImplementationOnce(
      () =>
        new Promise((_, reject) => {
          rejectSave = reject;
        }),
    );
    const wrapper = mountView();
    await flushPromises();
    await openCreate(wrapper);
    await fillCreate(wrapper);
    bodyButton("创建").click();
    bodyButton("创建").click();
    await flushPromises();
    expect(adminApiMock.createUser).toHaveBeenCalledOnce();
    const modal = wrapper.findComponent(NModal);
    expect(modal.props("closable")).toBe(false);
    expect(modal.props("maskClosable")).toBe(false);
    expect(modal.props("closeOnEsc")).toBe(false);
    expect(bodyButton("取消").disabled).toBe(true);
    rejectSave(new Error("interrupted"));
    await flushPromises();
    expect(modal.props("closable")).toBe(true);
    bodyButton("取消").click();
    await flushPromises();
    await openCreate(wrapper);
    expect(formField(wrapper, "username").findComponent(NInput).props("value")).toBe("");
    expect(formField(wrapper, "password").findComponent(NInput).props("value")).toBe("");
  });

  it.each([
    ["username", "alice-new"],
    ["email", "new@example.com"],
    ["role", "admin"],
    ["status", "disabled"],
    ["group_id", 2],
  ])(
    "clears the current session and returns to login after a self %s change",
    async (path, value) => {
      const auth = signInAs();
      adminApiMock.patchUser.mockResolvedValue(makeUser({ [path]: value }));
      const wrapper = mountView();
      await flushPromises();
      await openEdit(wrapper);
      if (typeof value === "string" && ["username", "email"].includes(path as string)) {
        await setText(wrapper, path as string, value);
      } else {
        await setChoice(wrapper, path as string, value);
      }
      expect(document.body.textContent).toContain(
        "修改自己的用户名、邮箱、角色、状态或用户组后，需要重新登录",
      );
      bodyButton("保存").click();
      await flushPromises();
      expect(auth.token).toBeNull();
      expect(auth.user).toBeNull();
      expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBeNull();
      expect(replaceRoute).toHaveBeenCalledWith("/login");
      expect(adminApiMock.listUsers).toHaveBeenCalledTimes(1);
    },
  );

  it("refreshes nickname and avatar in the current user without invalidating the session", async () => {
    const auth = signInAs();
    const updated = makeUser({
      display_name: "Alice renamed",
      avatar_config_version: 3,
      avatar_url: "https://weavatar.com/avatar/hash",
    });
    adminApiMock.patchUser.mockResolvedValue(updated);
    const wrapper = mountView();
    await flushPromises();
    await openEdit(wrapper);
    await setText(wrapper, "display_name", " Alice renamed ");
    bodyButton("保存").click();
    await flushPromises();
    expect(adminApiMock.patchUser).toHaveBeenCalledWith(1, { display_name: "Alice renamed" });
    expect(auth.user).toEqual(updated);
    expect(auth.token).toBe("current-session");
    expect(replaceRoute).not.toHaveBeenCalled();
  });

  it("does not invalidate a normalized no-op identity edit and allows clearing nickname", async () => {
    const initial = makeUser({ display_name: "Alice" });
    adminApiMock.listUsers.mockResolvedValue(makePage([initial]));
    const auth = signInAs(initial);
    adminApiMock.patchUser.mockResolvedValue(makeUser());
    const wrapper = mountView();
    await flushPromises();
    await openEdit(wrapper);
    await setText(wrapper, "username", " alice ");
    await setText(wrapper, "email", " ALICE@EXAMPLE.COM ");
    await setText(wrapper, "display_name", " ");
    bodyButton("保存").click();
    await flushPromises();
    expect(adminApiMock.patchUser).toHaveBeenCalledWith(1, { display_name: "" });
    expect(auth.user?.display_name).toBe("");
    expect(auth.token).toBe("current-session");
    expect(replaceRoute).not.toHaveBeenCalled();
  });

  it.each(["status", "group"])(
    "invalidates self sessions from the existing inline %s control",
    async (control) => {
      const auth = signInAs();
      adminApiMock.patchUser.mockResolvedValue(
        makeUser(control === "status" ? { status: "disabled" } : { group_id: 2 }),
      );
      const wrapper = mountView();
      await flushPromises();
      if (control === "status")
        wrapper.findAllComponents(NSwitch)[0].vm.$emit("update:value", false);
      else wrapper.findAllComponents(NSelect)[0].vm.$emit("update:value", 2);
      await flushPromises();
      expect(auth.token).toBeNull();
      expect(replaceRoute).toHaveBeenCalledWith("/login");
    },
  );

  it("accepts a fully normalized no-op edit without ending the current session", async () => {
    const auth = signInAs();
    adminApiMock.patchUser.mockResolvedValue(makeUser());
    const wrapper = mountView();
    await flushPromises();
    await openEdit(wrapper);
    await setText(wrapper, "username", " alice ");
    await setText(wrapper, "email", " ALICE@EXAMPLE.COM ");
    bodyButton("保存").click();
    await flushPromises();
    expect(adminApiMock.patchUser).toHaveBeenCalledExactlyOnceWith(1, {});
    expect(auth.token).toBe("current-session");
    expect(replaceRoute).not.toHaveBeenCalled();
    expect(wrapper.findComponent(NModal).props("show")).toBe(false);
  });

  it("keeps the current session when a self identity edit is rejected", async () => {
    const auth = signInAs();
    adminApiMock.patchUser.mockRejectedValue(new ApiError(30002, "duplicate", 409));
    const wrapper = mountView();
    await flushPromises();
    await openEdit(wrapper);
    await setText(wrapper, "email", "duplicate@example.com");
    bodyButton("保存").click();
    await flushPromises();
    expect(auth.user?.email).toBe("alice@example.com");
    expect(auth.token).toBe("current-session");
    expect(replaceRoute).not.toHaveBeenCalled();
    expect(wrapper.findComponent(NModal).props("show")).toBe(true);
  });

  it("does not invalidate the administrator when editing another account", async () => {
    const auth = signInAs(makeUser({ id: 99, role: "admin", username: "owner" }));
    adminApiMock.patchUser.mockResolvedValue(makeUser({ email: "new@example.com" }));
    const wrapper = mountView();
    await flushPromises();
    await openEdit(wrapper);
    await setText(wrapper, "email", "new@example.com");
    bodyButton("保存").click();
    await flushPromises();
    expect(auth.user?.username).toBe("owner");
    expect(auth.token).toBe("current-session");
    expect(replaceRoute).not.toHaveBeenCalled();
  });

  it("does not clear a newer login when an older self update completes", async () => {
    const auth = signInAs();
    let finish!: (user: AdminUserView) => void;
    adminApiMock.patchUser.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const wrapper = mountView();
    await flushPromises();
    await openEdit(wrapper);
    await setText(wrapper, "email", "new@example.com");
    bodyButton("保存").click();
    await flushPromises();
    auth.clear();
    auth.token = "new-session";
    auth.user = makeUser({ email: "new@example.com" });
    localStorage.setItem(TOKEN_STORAGE_KEY, "new-session");
    finish(makeUser({ email: "new@example.com" }));
    await flushPromises();
    expect(auth.token).toBe("new-session");
    expect(replaceRoute).not.toHaveBeenCalled();
    expect(adminApiMock.listUsers).toHaveBeenCalledTimes(1);
  });

  it("localizes an open account form when the language changes", async () => {
    const wrapper = mountView();
    await flushPromises();
    await openCreate(wrapper);
    i18n.global.locale.value = "en-US";
    await nextTick();
    expect(document.body.textContent).toContain("Create user");
    expect(document.body.textContent).toContain("Initial password");
    expect(document.body.textContent).toContain("Display name");
    expect(document.body.textContent).not.toContain("初始密码");
  });
});
