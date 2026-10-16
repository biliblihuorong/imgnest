import type { RouteRecordRaw } from "vue-router";

/** 系统设置的二级页面；新增设置页只需在这里加一项。 */
export const SETTINGS_SECTIONS = [
  {
    key: "general",
    name: "admin-settings-general",
    component: () => import("./GeneralSettingsView.vue"),
  },
  { key: "auth", name: "admin-settings-auth", component: () => import("./AuthSettingsView.vue") },
  {
    key: "access",
    name: "admin-settings-access",
    component: () => import("./AccessSettingsView.vue"),
  },
  { key: "mail", name: "admin-settings-mail", component: () => import("./MailSettingsView.vue") },
  {
    key: "plugins",
    name: "admin-settings-plugins",
    component: () => import("./PluginSettingsView.vue"),
  },
  {
    key: "backup",
    name: "admin-settings-backup",
    component: () => import("./BackupSettingsView.vue"),
  },
] as const;

export type SettingsSectionKey = (typeof SETTINGS_SECTIONS)[number]["key"];

export function settingsRoutes(): RouteRecordRaw[] {
  return SETTINGS_SECTIONS.map((section) => ({
    path: section.key,
    name: section.name,
    component: section.component,
    meta: { title: `admin.settings.sections.${section.key}.title` },
  }));
}
