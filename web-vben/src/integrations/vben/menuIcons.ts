import type { Component } from "vue";
import {
  ImagePlus,
  Inbox,
  InspectionPanel,
  LayoutGrid,
  List,
  LockKeyhole,
  Settings,
  UserRoundPen,
} from "@vben/icons";

/** navigation.ts 里菜单 icon 名到组件的映射；两套外壳共用。 */
export const menuIcons: Record<string, Component> = {
  ImagePlus,
  Inbox,
  InspectionPanel,
  LayoutGrid,
  List,
  LockKeyhole,
  Settings,
  UserRoundPen,
};
