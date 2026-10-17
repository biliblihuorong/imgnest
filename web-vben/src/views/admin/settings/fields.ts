import type { AdminSettings } from "@/api/admin";
import type { DocTopicName } from "@/lib/docs";

export type SettingsFieldKind = "text" | "switch" | "number" | "group" | "avatar";

/** 一个站点设置项：key 对应 AdminSettings 字段，label/hint 为 admin.settings 下的文案键。 */
export interface SettingsField {
  key: keyof AdminSettings;
  kind: SettingsFieldKind;
  label: string;
  hint: string;
  placeholder?: string;
  /** 标签旁「?」打开的文档，见 @/lib/docs。 */
  doc?: DocTopicName;
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
