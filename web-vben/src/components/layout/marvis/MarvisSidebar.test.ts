import { mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { beforeEach, expect, it, vi } from "vitest";
import type { UserView } from "@/api/types";
import { useAuthStore } from "@/stores/auth";
import { useSiteStore } from "@/stores/site";
import MarvisSidebar from "./MarvisSidebar.vue";
import { shellTestRouter, testUser } from "./testing";

vi.mock("@/api/site", () => ({ fetchSite: vi.fn().mockRejectedValue(new Error("offline")) }));

beforeEach(() => {
  localStorage.clear();
});

async function mountSidebar(options: {
  user?: UserView;
  gallery?: boolean;
  collapsed?: boolean;
  path?: string;
  slots?: Record<string, string>;
}) {
  const pinia = createPinia();
  const auth = useAuthStore(pinia);
  const site = useSiteStore(pinia);
  if (options.user) {
    auth.token = "token";
    auth.user = options.user;
  }
  site.galleryEnabled = options.gallery ?? false;
  const router = shellTestRouter();
  await router.push(options.path ?? (options.user ? "/upload" : "/gallery"));
  const wrapper = mount(MarvisSidebar, {
    props: { collapsed: options.collapsed ?? false },
    slots: options.slots,
    global: { plugins: [pinia, router] },
  });
  const links = () => wrapper.findAll("nav a").map((a) => a.attributes("href"));
  return { wrapper, router, links };
}

it("renders the member menu from workspaceMenus", async () => {
  const { links } = await mountSidebar({ user: testUser("user") });
  expect(links()).toEqual(["/upload", "/images", "/albums", "/dashboard", "/tokens"]);
});

it("groups admin children under a labelled section", async () => {
  const { wrapper, links } = await mountSidebar({ user: testUser("admin") });
  expect(wrapper.find('[data-testid="sidebar-group-/admin"]').text()).toBe("站点管理");
  expect(links()).toEqual(
    expect.arrayContaining(["/admin/users", "/admin/groups", "/admin/images"]),
  );
  expect(links()).not.toContain("/admin");
});

it("shows only gallery and a login button to anonymous visitors", async () => {
  const { wrapper, links } = await mountSidebar({ gallery: true });
  expect(links()).toEqual(["/gallery"]);
  expect(wrapper.find('[data-testid="sidebar-login"]').exists()).toBe(true);
  // 匿名访客也要能切换深浅色、语言和布局。
  expect(wrapper.find('[data-testid="sidebar-settings"]').exists()).toBe(true);
  expect(wrapper.find('[data-testid="sidebar-logout"]').exists()).toBe(false);
  expect(wrapper.find('[data-testid="sidebar-search"]').exists()).toBe(false);
});

it("marks the active route with aria-current", async () => {
  const { wrapper } = await mountSidebar({ user: testUser("user"), path: "/images" });
  expect(wrapper.find('a[href="/images"]').attributes("aria-current")).toBe("page");
  expect(wrapper.find('a[href="/upload"]').attributes("aria-current")).toBeUndefined();
});

it("hides labels when collapsed but keeps accessible names", async () => {
  const { wrapper } = await mountSidebar({ user: testUser("user"), collapsed: true });
  const upload = wrapper.find('a[href="/upload"]');
  expect(upload.attributes("aria-label")).toBe("上传图片");
  expect(upload.find(".mv-nav__text").exists()).toBe(false);
});

it("emits search, settings, logout and navigate intents without acting on them", async () => {
  const { wrapper } = await mountSidebar({ user: testUser("user") });
  await wrapper.find('[data-testid="sidebar-search"]').trigger("click");
  await wrapper.find('[data-testid="sidebar-settings"]').trigger("click");
  await wrapper.find('[data-testid="sidebar-logout"]').trigger("click");
  await wrapper.find('a[href="/images"]').trigger("click");
  expect(wrapper.emitted("open-search")).toHaveLength(1);
  expect(wrapper.emitted("open-settings")).toHaveLength(1);
  expect(wrapper.emitted("logout")).toHaveLength(1);
  expect(wrapper.emitted("navigate")).toHaveLength(1);
});

it("renders the albums slot under the albums entry only when expanded", async () => {
  const slots = { albums: '<div data-testid="albums-slot" />' };
  const open = await mountSidebar({ user: testUser("user"), slots });
  expect(open.wrapper.find('[data-testid="albums-slot"]').exists()).toBe(true);
  const closed = await mountSidebar({ user: testUser("user"), slots, collapsed: true });
  expect(closed.wrapper.find('[data-testid="albums-slot"]').exists()).toBe(false);
});
