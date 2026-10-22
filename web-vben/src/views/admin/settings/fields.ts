import type { AdminSettings } from "@/api/admin";
import type { DocTopicName } from "@/lib/docs";

export type SettingsFieldKind =
  "text" | "textarea" | "switch" | "number" | "group" | "avatar" | "robots";

/** 一个站点设置项：key 对应 AdminSettings 字段，label/hint 为 admin.settings 下的文案键。 */
export interface SettingsField {
  key: keyof AdminSettings;
  kind: SettingsFieldKind;
  label: string;
  hint: string;
  placeholder?: string;
  /** 标签旁「?」打开的文档，见 @/lib/docs。 */
  doc?: DocTopicName;
  /** 只在条件成立时显示；隐藏时字段仍随本页一起保存。 */
  visible?: (settings: AdminSettings) => boolean;
}

export const SITE_NAME: SettingsField = {
  key: "site_name",
  kind: "text",
  label: "siteName",
  hint: "siteNameHint",
};
export const AVATAR_PROVIDER: SettingsField = {
  key: "avatar_provider",
  kind: "avatar",
  label: "avatarProvider",
  hint: "avatarProviderHint",
};
export const API_ENABLED: SettingsField = {
  key: "api_enabled",
  kind: "switch",
  label: "api",
  hint: "apiHint",
};
export const TRASH_DAYS: SettingsField = {
  key: "trash_days",
  kind: "number",
  label: "retention",
  hint: "retentionHint",
};
export const REGISTRATION: SettingsField = {
  key: "registration_enabled",
  kind: "switch",
  label: "registration",
  hint: "registrationHint",
};
export const DEFAULT_GROUP: SettingsField = {
  key: "default_group_id",
  kind: "group",
  label: "defaultGroup",
  hint: "defaultGroupHint",
  placeholder: "defaultGroupPlaceholder",
};
export const EMAIL_VERIFICATION: SettingsField = {
  key: "email_verification_enabled",
  kind: "switch",
  label: "emailVerification",
  hint: "emailVerificationHint",
};
export const GUEST_UPLOAD: SettingsField = {
  key: "guest_upload_enabled",
  kind: "switch",
  label: "guestUpload",
  hint: "guestUploadHint",
  doc: "guestUpload",
};
export const GUEST_GROUP: SettingsField = {
  key: "guest_group_id",
  kind: "group",
  label: "guestGroup",
  hint: "guestGroupHint",
  placeholder: "guestGroupPlaceholder",
};
export const GALLERY: SettingsField = {
  key: "gallery_enabled",
  kind: "switch",
  label: "gallery",
  hint: "galleryHint",
  doc: "gallery",
};
export const GALLERY_PUBLIC_ALBUMS_ONLY: SettingsField = {
  key: "gallery_public_albums_only",
  kind: "switch",
  label: "galleryPublicAlbumsOnly",
  hint: "galleryPublicAlbumsOnlyHint",
};
export const ROBOTS_MODE: SettingsField = {
  key: "robots_mode",
  kind: "robots",
  label: "robotsMode",
  hint: "robotsModeHint",
};
export const ROBOTS_CUSTOM: SettingsField = {
  key: "robots_custom",
  kind: "textarea",
  label: "robotsCustom",
  hint: "robotsCustomHint",
  placeholder: "robotsCustomPlaceholder",
  visible: (settings) => settings.robots_mode === "custom",
};
export const SITEMAP: SettingsField = {
  key: "sitemap_enabled",
  kind: "switch",
  label: "sitemap",
  hint: "sitemapHint",
};
