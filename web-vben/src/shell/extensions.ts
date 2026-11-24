import type { Component } from "vue";
import type { CurrentUser } from "@/api/types";

/**
 * 外壳上可以挂项目组件的位置。项目在 app.hooks.ts 的 shellExtensions 里填，外壳只认这个形状，
 * 不认识具体业务。都不填时外壳就是模板的默认样子。
 */
export interface ShellExtensions {
  /**
   * 替换 Ctrl/⌘+K 命令面板（默认只按名称搜页面）。组件接收 v-model:show，
   * 外层用 shell/marvis/MarvisDialog.vue 即可得到同样的弹窗外观；页面列表可用 shell/navigation.ts 的 leafMenus。
   */
  commandPalette?: Component;
  /** 侧栏搜索按钮的文字（i18n key），配合自定义命令面板使用；默认 shell.search。 */
  searchLabel?: string;
  /**
   * 插在 Marvis 侧栏某个菜单项下方的组件（例如可展开的子列表），侧栏折叠时不渲染。
   * 接收 prop `item`（当前菜单项，自己按 item.path 判断要不要显示），可 emit `navigate`（窄屏时收起抽屉）。
   */
  sidebarItemExtra?: Component;
  /** 经典布局右上角账号菜单里额外的链接，排在「账号」之后。 */
  userMenu?: (user: CurrentUser) => UserMenuLink[];
}

export interface UserMenuLink {
  /** i18n key。 */
  label: string;
  path: string;
}
