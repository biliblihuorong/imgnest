import { request } from "./client";

export type MailEncryption = "tls" | "starttls" | "none";

/** SMTP 设置；密码只写不读，password_configured 表示已保存。 */
export interface MailSettings {
  enabled: boolean;
  host: string;
  port: number;
  encryption: MailEncryption;
  username: string;
  password_configured: boolean;
  from_address: string;
  from_name: string;
  /** 未配置主密钥时为 false，此时不能保存密码。 */
  configuration_available: boolean;
}

export interface MailSettingsPayload {
  enabled: boolean;
  host: string;
  port: number;
  encryption: MailEncryption;
  username: string;
  /** 省略表示沿用已保存的密码。 */
  password?: string;
  clear_password?: boolean;
  from_address: string;
  from_name: string;
}

const JSON_HEADERS = { "Content-Type": "application/json" };

export function getMailSettings(): Promise<MailSettings> {
  return request<MailSettings>("/api/admin/mail");
}

export function saveMailSettings(payload: MailSettingsPayload): Promise<MailSettings> {
  return request<MailSettings>("/api/admin/mail", {
    method: "PUT",
    headers: JSON_HEADERS,
    body: JSON.stringify(payload),
  });
}

/** 用已保存的设置发一封测试邮件；to 为空时发给当前管理员。 */
export function sendTestMail(to: string): Promise<null> {
  return request<null>("/api/admin/mail/test", {
    method: "POST",
    headers: JSON_HEADERS,
    body: JSON.stringify({ to }),
  });
}
