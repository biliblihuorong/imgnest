import { describe, expect, it } from "vitest";
import { workspaceMenus } from "./navigation";

describe("Vben 工作区导航权限", () => {
  it("身份未恢复时不展示业务菜单", () => {
    expect(workspaceMenus(undefined)).toEqual([]);
  });
  it("普通用户只看到已有的上传、图片和账户功能", () => {
    expect(workspaceMenus("user").map((menu) => menu.path)).toEqual([
      "/upload",
      "/images",
      "/albums",
      "/dashboard",
      "/tokens",
    ]);
  });
  it("管理员看到六个管理页和真实 M5 相册", () => {
    const admin = workspaceMenus("admin").find((menu) => menu.path === "/admin");
    expect(admin?.children?.map((menu) => menu.path)).toEqual([
      "/admin/users",
      "/admin/groups",
      "/admin/storages",
      "/admin/policies",
      "/admin/settings",
      "/admin/images",
    ]);
    expect(workspaceMenus("admin").some((menu) => menu.path === "/albums")).toBe(true);
  });
  it("调用者编辑菜单不会污染下次登录的权限视图", () => {
    const menus = workspaceMenus("admin");
    menus.pop();
    expect(workspaceMenus("admin")).toHaveLength(6);
    expect(workspaceMenus("user")).toHaveLength(5);
  });
  it("仅开放画廊时向匿名用户展示公开入口", () => {
    expect(workspaceMenus(undefined, true).map((m) => m.path)).toEqual(["/gallery"]);
    expect(workspaceMenus("user", false).some((m) => m.path === "/gallery")).toBe(false);
  });
});
