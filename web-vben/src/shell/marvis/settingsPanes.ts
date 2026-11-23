import { Palette } from "@vben/icons";
import { defineAsyncComponent, markRaw, type Component } from "vue";

export interface SettingsPane {
  key: string;
  /** i18n key。 */
  label: string;
  icon: Component;
  component: Component;
  /** 需要登录才显示。 */
  requiresAuth?: boolean;
}

/**
 * 设置弹窗（侧栏齿轮、经典布局顶栏齿轮）的分类。ImgNest 只放外观与语言；
 * 资料、密码与 API Token 在账户页（appConfig.accountPath），弹窗底部有入口。
 */
export const settingsPanes: SettingsPane[] = [
  {
    key: "appearance",
    label: "shell.settings.appearance",
    icon: markRaw(Palette),
    component: markRaw(defineAsyncComponent(() => import("./AppearancePane.vue"))),
  },
];
