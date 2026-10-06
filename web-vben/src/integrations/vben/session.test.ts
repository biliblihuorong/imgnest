import { flushPromises, mount } from "@vue/test-utils";
import { h, onUnmounted } from "vue";
import { RouterView } from "vue-router";
import { createPinia } from "pinia";
import { createMemoryHistory, createRouter } from "vue-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useAccessStore, useTabbarStore, useUserStore } from "@vben/stores";
import { TOKEN_STORAGE_KEY } from "@/api/client";
import type { UserView } from "@/api/types";
import { useAuthStore } from "@/stores/auth";
import { connectVbenSession } from "./session";

const admin: UserView = {
  id: 1,
  username: "admin",
  email: "admin@example.test",
  role: "admin",
  status: "enabled",
  group_id: 1,
  used_bytes: 0,
  created_at: "2026-10-05T00:00:00Z",
  display_name: "",
  avatar_provider: "weavatar",
  avatar_url: "https://weavatar.com/avatar/abc?s=160&d=404",
  avatar_config_version: 0,
};
let stop: (() => void) | undefined;
beforeEach(() => localStorage.clear());
afterEach(() => stop?.());

function setup(user: UserView | null = admin) {
  localStorage.setItem(TOKEN_STORAGE_KEY, "local-fixture-token");
  const pinia = createPinia();
  const auth = useAuthStore(pinia);
  auth.user = user;
  const view = { template: "<div />" };
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/upload", component: view },
      { path: "/admin", component: view, meta: { admin: true, title: "common.nav.admin" } },
      { path: "/login", component: view, meta: { bare: true, title: "common.login" } },
    ],
  });
  stop = connectVbenSession(pinia, router);
  return {
    auth,
    access: useAccessStore(pinia),
    user: useUserStore(pinia),
    tabs: useTabbarStore(pinia),
  };
}

describe("ImageNest → Vben UI 会话投影", () => {
  it("只恢复 Token 时不提前显示管理菜单", () => {
    const { access, user } = setup(null);
    expect(access.accessMenus).toEqual([]);
    expect(user.userInfo).toBeNull();
  });
  it("真实角色决定菜单，同时不复制或额外持久化凭证", () => {
    const { access, user } = setup();
    expect(access.accessMenus.find((menu) => menu.path === "/admin")?.children).toHaveLength(6);
    expect(user.userInfo?.roles).toEqual(["admin"]);
    expect(access.accessToken).toBeNull();
    expect(access.refreshToken).toBeNull();
    expect(Object.keys(localStorage)).toEqual([TOKEN_STORAGE_KEY]);
  });
  it("登出清除导航、身份与旧标签页，保留原有 token 清除契约", () => {
    const { auth, access, user, tabs } = setup();
    tabs.addTab({
      path: "/admin/users",
      fullPath: "/admin/users",
      name: "admin-users",
      meta: { title: "用户管理" },
      matched: [],
    } as never);
    auth.clear();
    expect(access.accessMenus).toEqual([]);
    expect(user.userInfo).toBeNull();
    expect(tabs.getTabs).toHaveLength(0);
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBeNull();
  });
  it("同一账号失去管理员身份后删除管理菜单和访问路由", () => {
    const { auth, access } = setup();
    auth.user = { ...admin, role: "user" };
    expect(access.accessMenus.map((menu) => menu.path)).toEqual([
      "/upload",
      "/images",
      "/albums",
      "/dashboard",
      "/tokens",
    ]);
    expect(access.accessRoutes.some((route) => route.meta?.admin)).toBe(false);
  });
});

it("demotion unmounts the already open administrative view", async () => {
  const pinia = createPinia();
  const auth = useAuthStore(pinia);
  auth.token = "fixture-admin";
  auth.user = admin;
  const unmounted = vi.fn();
  const adminView = {
    setup() {
      onUnmounted(unmounted);
      return () => h("div", { "data-sensitive": "true" }, "Private administration");
    },
  };
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      {
        path: "/admin/users",
        component: adminView,
        meta: { admin: true, title: "common.nav.users" },
      },
      {
        path: "/forbidden",
        component: { template: "<p>Forbidden</p>" },
        meta: { title: "common.forbidden" },
      },
    ],
  });
  await router.push("/admin/users");
  stop = connectVbenSession(pinia, router);
  const wrapper = mount(RouterView, { global: { plugins: [pinia, router] } });
  expect(wrapper.find("[data-sensitive]").exists()).toBe(true);
  auth.user = { ...admin, role: "user" };
  await flushPromises();
  expect(router.currentRoute.value.path).toBe("/forbidden");
  expect(wrapper.find("[data-sensitive]").exists()).toBe(false);
  expect(unmounted).toHaveBeenCalledTimes(1);
  wrapper.unmount();
});

it("session clearing immediately unmounts privileged content", async () => {
  const pinia = createPinia();
  const auth = useAuthStore(pinia);
  auth.token = "fixture-admin";
  auth.user = admin;
  const unmounted = vi.fn();
  const adminView = {
    setup() {
      onUnmounted(unmounted);
      return () => h("div", { "data-sensitive": "true" }, "Private administration");
    },
  };
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      {
        path: "/admin/users",
        component: adminView,
        meta: { admin: true, title: "common.nav.users" },
      },
      {
        path: "/login",
        component: { template: "<p>Login</p>" },
        meta: { bare: true, title: "common.login" },
      },
    ],
  });
  await router.push("/admin/users");
  stop = connectVbenSession(pinia, router);
  const wrapper = mount(RouterView, { global: { plugins: [pinia, router] } });
  expect(wrapper.find("[data-sensitive]").exists()).toBe(true);
  auth.clear();
  await flushPromises();
  expect(router.currentRoute.value.path).toBe("/login");
  expect(wrapper.find("[data-sensitive]").exists()).toBe(false);
  expect(unmounted).toHaveBeenCalledTimes(1);
  wrapper.unmount();
});
