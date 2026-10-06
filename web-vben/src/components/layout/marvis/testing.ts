import { createMemoryHistory, createRouter, type Router } from "vue-router";
import type { UserView } from "@/api/types";

/** 测试专用：覆盖工作区菜单全部路径的内存路由。 */
export function shellTestRouter(): Router {
  const view = { template: "<div />" };
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      "/upload",
      "/images",
      "/albums",
      "/albums/:id",
      "/dashboard",
      "/tokens",
      "/gallery",
      "/login",
      "/account/settings",
      "/admin/users",
      "/admin/groups",
      "/admin/storages",
      "/admin/policies",
      "/admin/settings",
      "/admin/images",
    ].map((path) => ({ path, component: view })),
  });
}

export function testUser(role: UserView["role"], id = 1): UserView {
  return {
    id,
    username: `user${id}`,
    display_name: `User ${id}`,
    email: `user${id}@example.test`,
    role,
  } as UserView;
}
