import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it } from "vitest";
import type { CurrentUser, UserView } from "@/api/types";
import { homePath } from "@/app.hooks";
import { shellMenus } from "@/shell/navigation";
import { useSiteStore } from "@/stores/site";
import { useSystemStore } from "@/stores/system";
import type { SystemVersion } from "@/api/system";

function account(role: UserView["role"]): CurrentUser {
  return {
    id: role === "admin" ? 1 : 2,
    role,
    username: role,
    email: `${role}@example.com`,
    status: "enabled",
    display_name: "",
    group_id: 1,
    used_bytes: 0,
    created_at: "2026-01-01T00:00:00Z",
    avatar_provider: "weavatar",
    avatar_url: null,
    avatar_config_version: 1,
  };
}

function menusFor(
  role: UserView["role"] | undefined,
  galleryEnabled = false,
  updateAvailable = false,
) {
  useSiteStore().galleryEnabled = galleryEnabled;
  useSystemStore().version = updateAvailable ? ({ update_available: true } as SystemVersion) : null;
  return shellMenus(role ? account(role) : null);
}

beforeEach(() => setActivePinia(createPinia()));

describe("工作区菜单（app.hooks.ts 的 resolveMenus + 外壳的权限过滤）", () => {
  it("未登录且没开画廊时不展示菜单", () => {
    expect(menusFor(undefined)).toEqual([]);
  });
  it("普通用户只看到已有的上传、图片和账户功能", () => {
    expect(menusFor("user").map((menu) => menu.path)).toEqual([
      "/upload",
      "/images",
      "/albums",
      "/dashboard",
      "/account",
    ]);
  });
  it("管理员看到全部管理页和相册", () => {
    const admin = menusFor("admin").find((menu) => menu.path === "/admin");
    expect(admin?.children?.map((menu) => menu.path)).toEqual([
      "/admin/overview",
      "/admin/users",
      "/admin/groups",
      "/admin/storages",
      "/admin/policies",
      "/admin/settings",
      "/admin/images",
      "/admin/audit-logs",
    ]);
    expect(menusFor("admin").some((menu) => menu.path === "/albums")).toBe(true);
  });
  it("调用者编辑菜单不会污染下次登录的权限视图", () => {
    const menus = menusFor("admin");
    menus.pop();
    expect(menusFor("admin")).toHaveLength(6);
    expect(menusFor("user")).toHaveLength(5);
  });
  it("仅开放画廊时向匿名用户展示公开入口", () => {
    expect(menusFor(undefined, true).map((m) => m.path)).toEqual(["/gallery"]);
    expect(menusFor("user", false).some((m) => m.path === "/gallery")).toBe(false);
    expect(menusFor("user", true).some((m) => m.path === "/gallery")).toBe(true);
  });
  it("有新版本时只在管理员的系统设置上挂小红点", () => {
    const admin = menusFor("admin", false, true).find((menu) => menu.path === "/admin");
    expect(admin?.dot).toBe(true);
    expect(admin?.children?.filter((menu) => menu.dot).map((menu) => menu.path)).toEqual([
      "/admin/settings",
    ]);
    expect(menusFor("user", false, true).some((menu) => menu.dot)).toBe(false);
    expect(menusFor("admin").find((menu) => menu.path === "/admin")?.dot).toBeUndefined();
  });
});

describe("落地页", () => {
  it("管理员进仪表盘，普通用户进上传页，未登录点 Logo 去画廊", () => {
    expect(homePath(account("admin"))).toBe("/dashboard");
    expect(homePath(account("user"))).toBe("/upload");
    expect(homePath(null)).toBe("/gallery");
  });
});
