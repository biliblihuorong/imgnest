import { enableAutoUnmount, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createPinia, setActivePinia, type Pinia } from "pinia";
import { createMemoryHistory, createRouter, type Router } from "vue-router";
import type { UserView } from "@/api/types";
import { useAuthStore } from "@/stores/auth";
import { useSiteStore } from "@/stores/site";
import AppLayout from "./AppLayout.vue";

vi.mock("@/api/site", () => ({
  fetchSite: vi.fn(),
}));

enableAutoUnmount(afterEach);

const stubView = { template: "<div />" };

let pinia: Pinia;
let router: Router;

function makeUser(overrides: Partial<UserView> = {}): UserView {
  return {
    id: 1,
    group_id: 1,
    username: "alice",
    email: "alice@example.com",
    role: "user",
    status: "enabled",
    used_bytes: 0,
    created_at: "2026-10-05T00:00:00Z",
    ...overrides,
  };
}

function mountLayout() {
  return mount(AppLayout, { global: { plugins: [pinia, router] } });
}

function menuLabels(wrapper: ReturnType<typeof mountLayout>): string[] {
  // jsdom 宽度为 0，responsive 菜单恒渲染「···」溢出项，过滤掉。
  return wrapper
    .findAll(".n-menu-item-content")
    .map((n) => n.text().trim())
    .filter((label) => label !== "···");
}

function findButton(wrapper: ReturnType<typeof mountLayout>, text: string) {
  return wrapper.findAll("button").find((b) => b.text() === text);
}

beforeEach(async () => {
  vi.clearAllMocks();
  pinia = createPinia();
  setActivePinia(pinia);
  router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/", component: stubView },
      { path: "/login", component: stubView },
      { path: "/upload", component: stubView },
      { path: "/gallery", component: stubView },
    ],
  });
  router.push("/");
  await router.isReady();
  // 站点信息预置为已加载，隔离 AppLayout 里 ensureLoaded 的真实请求。
  const site = useSiteStore();
  site.loaded = true;
  site.galleryEnabled = false;
});

describe("AppLayout", () => {
  it("匿名态：显示登录按钮、无用户下拉触发按钮、菜单仍渲染且不崩溃", () => {
    const wrapper = mountLayout();

    expect(findButton(wrapper, "账户")).toBeUndefined();
    expect(findButton(wrapper, "登录")).toBeDefined();
    expect(menuLabels(wrapper)).toEqual(["上传", "图片", "Token"]);
  });

  it("登录态菜单出现相册入口，匿名态不出现", () => {
    const anonymous = mountLayout();
    expect(menuLabels(anonymous)).not.toContain("相册");
    anonymous.unmount();

    useAuthStore().user = makeUser();
    const loggedIn = mountLayout();
    expect(menuLabels(loggedIn)).toEqual(["上传", "图片", "相册", "Token"]);
  });

  it("匿名态点击登录按钮跳转 /login", async () => {
    const pushSpy = vi.spyOn(router, "push").mockResolvedValue(undefined);
    const wrapper = mountLayout();

    await findButton(wrapper, "登录")!.trigger("click");

    expect(pushSpy).toHaveBeenCalledWith("/login");
  });

  it("登录态：显示用户名与用户下拉触发按钮，无登录按钮", () => {
    useAuthStore().user = makeUser({ username: "alice" });
    const wrapper = mountLayout();

    // 用户下拉的触发按钮是渲染用户名的 NButton。
    expect(findButton(wrapper, "alice")).toBeDefined();
    expect(wrapper.text()).toContain("alice");
    expect(findButton(wrapper, "登录")).toBeUndefined();
  });

  it("galleryEnabled=true 时菜单出现画廊项（匿名与登录均显示）", () => {
    useSiteStore().galleryEnabled = true;

    const anonymous = mountLayout();
    expect(menuLabels(anonymous)).toContain("画廊");
    anonymous.unmount();

    useAuthStore().user = makeUser();
    const loggedIn = mountLayout();
    expect(menuLabels(loggedIn)).toContain("画廊");
  });

  it("galleryEnabled=false 时菜单不出现画廊项", () => {
    useAuthStore().user = makeUser();
    const loggedIn = mountLayout();
    expect(menuLabels(loggedIn)).toEqual(["上传", "图片", "相册", "Token"]);
    loggedIn.unmount();

    useAuthStore().user = null;
    const anonymous = mountLayout();
    expect(menuLabels(anonymous)).toEqual(["上传", "图片", "Token"]);
    expect(menuLabels(anonymous)).not.toContain("画廊");
  });

  it("点击画廊菜单项跳转 /gallery", async () => {
    useSiteStore().galleryEnabled = true;
    const pushSpy = vi.spyOn(router, "push").mockResolvedValue(undefined);
    const wrapper = mountLayout();

    const galleryItem = wrapper
      .findAll(".n-menu-item-content")
      .find((n) => n.text() === "画廊");
    expect(galleryItem).toBeDefined();
    await galleryItem!.trigger("click");

    expect(pushSpy).toHaveBeenCalledWith("/gallery");
  });
});
