import type { MenuRecordRaw } from "@vben/types";
import { $t } from "@vben/locales";
import { markRaw } from "vue";
import type { CurrentUser } from "@/api/types";
import type { MenuItem } from "@/app.config";
import { canAccess, resolveMenus } from "@/app.hooks";

export type { MenuItem };

/**
 * 按 app.hooks.ts 的 canAccess 过滤菜单树：未登录只留 public 项，子项全被过滤掉的分组也去掉。
 * 返回新数组，调用方修改不会影响下次计算。
 */
export function visibleMenus(
  user: CurrentUser | null | undefined,
  source: MenuItem[] = resolveMenus(user ?? null),
): MenuItem[] {
  const result: MenuItem[] = [];
  for (const item of source) {
    if (user ? !canAccess(user, item) : !item.public) continue;
    const children = item.children ? visibleMenus(user, item.children) : undefined;
    if (item.children && children!.length === 0) continue;
    result.push({ ...item, ...(children ? { children } : {}) });
  }
  return result;
}

/** 当前用户可见的菜单树（来源是 app.hooks.ts 的 resolveMenus；在响应式上下文里调用会跟着刷新）。 */
export function shellMenus(user: CurrentUser | null | undefined): MenuItem[] {
  return visibleMenus(user);
}

/** 叶子菜单（命令面板的“页面”列表用）。 */
export function leafMenus(user: CurrentUser | null | undefined): MenuItem[] {
  const leaves: MenuItem[] = [];
  const walk = (items: MenuItem[]) => {
    for (const item of items) {
      if (item.children) walk(item.children);
      else leaves.push(item);
    }
  };
  walk(shellMenus(user));
  return leaves;
}

/** 转成 Vben 经典布局的菜单格式。 */
export function toVbenMenu(menu: MenuItem): MenuRecordRaw {
  return {
    name: $t(menu.name),
    path: menu.path,
    icon: menu.icon ? markRaw(menu.icon) : undefined,
    ...(menu.dot ? { badgeType: "dot" as const, badgeVariants: "destructive" } : {}),
    children: menu.children?.map(toVbenMenu),
  };
}
