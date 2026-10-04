import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { me } from "@/api/auth";
import { TOKEN_STORAGE_KEY } from "@/api/client";
import type { UserView } from "@/api/types";
import { useAuthStore } from "@/stores/auth";
import { ADMIN_ROUTER_NAMES, setupRouter } from "./index";

// 守卫测试只关心导航行为：stub 掉涉事视图模块，避免在测试阶段加载 naive-ui
vi.mock("@/api/auth", () => ({
  login: vi.fn(),
  logout: vi.fn(),
  register: vi.fn(),
  me: vi.fn(),
  changePassword: vi.fn(),
}));
vi.mock("@/views/LoginView.vue", () => ({
  default: { name: "LoginViewStub", template: "<div />" },
}));
vi.mock("@/components/admin/AdminLayout.vue", () => ({
  default: { name: "AdminLayoutStub", template: "<div />" },
}));
vi.mock("@/views/ForbiddenView.vue", () => ({
  default: { name: "ForbiddenViewStub", template: "<div />" },
}));
vi.mock("@/views/admin/UsersView.vue", () => ({
  default: { name: "AdminUsersViewStub", template: "<div />" },
}));
vi.mock("@/views/admin/GroupsView.vue", () => ({
  default: { name: "AdminGroupsViewStub", template: "<div />" },
}));
vi.mock("@/views/admin/StoragesView.vue", () => ({
  default: { name: "AdminStoragesViewStub", template: "<div />" },
}));
vi.mock("@/views/admin/PoliciesView.vue", () => ({
  default: { name: "AdminPoliciesViewStub", template: "<div />" },
}));
vi.mock("@/views/admin/SettingsView.vue", () => ({
  default: { name: "AdminSettingsViewStub", template: "<div />" },
}));
vi.mock("@/views/admin/ImagesAdminView.vue", () => ({
  default: { name: "AdminImagesViewStub", template: "<div />" },
}));

const meMock = vi.mocked(me);

const adminUser: UserView = {
  id: 1,
  group_id: 1,
  username: "root",
  email: "root@imgnest.local",
  role: "admin",
  status: "enabled",
  used_bytes: 0,
  created_at: "2026-10-04T00:00:00Z",
};

const plainUser: UserView = { ...adminUser, id: 2, username: "alice", role: "user" };

describe("admin 路由守卫", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.resetAllMocks();
    setActivePinia(createPinia());
  });

  it("未登录访问 /admin/users 跳转登录并携带 redirect，不触发 me()", async () => {
    const router = setupRouter();
    await router.push("/admin/users");
    await router.isReady();

    expect(router.currentRoute.value.path).toBe("/login");
    expect(router.currentRoute.value.query.redirect).toBe("/admin/users");
    expect(meMock).not.toHaveBeenCalled();
  });

  it("刷新后 user 未加载：守卫等待 restore()，admin 放行并填充 user", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|admin-token");
    meMock.mockResolvedValueOnce(adminUser);

    const router = setupRouter();
    await router.push("/admin/users");
    await router.isReady();

    expect(router.currentRoute.value.path).toBe("/admin/users");
    expect(router.currentRoute.value.name).toBe(ADMIN_ROUTER_NAMES.adminUsers);
    expect(meMock).toHaveBeenCalledTimes(1);
    expect(useAuthStore().user?.role).toBe("admin");
  });

  it("已登录但 role 不是 admin：重定向 /forbidden，不进 admin 页面", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "8|user-token");
    meMock.mockResolvedValueOnce(plainUser);

    const router = setupRouter();
    await router.push("/admin/users");
    await router.isReady();

    expect(router.currentRoute.value.path).toBe("/forbidden");
    expect(router.currentRoute.value.name).toBe(ADMIN_ROUTER_NAMES.forbidden);
    expect(router.currentRoute.value.matched.some((r) => r.meta.admin)).toBe(false);
  });

  it("user 已加载且是 admin：直接放行，不再调用 me()", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|admin-token");
    useAuthStore().user = adminUser;

    const router = setupRouter();
    await router.push("/admin/groups");
    await router.isReady();

    expect(router.currentRoute.value.path).toBe("/admin/groups");
    expect(meMock).not.toHaveBeenCalled();
  });

  it("token 失效（restore 失败清空登录态）：按未登录跳转登录页", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "9|expired-token");
    meMock.mockRejectedValueOnce(new Error("401"));

    const router = setupRouter();
    await router.push("/admin/settings");
    await router.isReady();

    expect(router.currentRoute.value.path).toBe("/login");
    expect(router.currentRoute.value.query.redirect).toBe("/admin/settings");
    expect(useAuthStore().token).toBeNull();
  });

  it("/admin 重定向到 /admin/users", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|admin-token");
    useAuthStore().user = adminUser;

    const router = setupRouter();
    await router.push("/admin");
    await router.isReady();

    expect(router.currentRoute.value.path).toBe("/admin/users");
  });
});
