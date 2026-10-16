import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { NMessageProvider, NSelect } from "naive-ui";
import { h } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/api/client";
import * as api from "@/api/mail";
import type { MailSettings } from "@/api/mail";
import MailSettingsCard from "./MailSettingsCard.vue";

vi.mock("@/api/mail", () => ({
  getMailSettings: vi.fn(),
  saveMailSettings: vi.fn(),
  sendTestMail: vi.fn(),
}));
vi.stubGlobal(
  "ResizeObserver",
  class {
    observe = vi.fn();
    unobserve = vi.fn();
    disconnect = vi.fn();
  },
);
enableAutoUnmount(afterEach);
const apiMock = vi.mocked(api);
const saved: MailSettings = {
  enabled: true,
  host: "smtp.example.test",
  port: 465,
  encryption: "tls",
  username: "noreply@example.test",
  password_configured: true,
  from_address: "noreply@example.test",
  from_name: "ImgNest",
  configuration_available: true,
};

async function render(view = saved) {
  apiMock.getMailSettings.mockResolvedValue(view);
  const wrapper = mount(() => h(NMessageProvider, () => h(MailSettingsCard)));
  await flushPromises();
  return wrapper;
}
function input(wrapper: Awaited<ReturnType<typeof render>>, label: string) {
  return wrapper.get(`input[aria-label="${label}"]`);
}

beforeEach(() => vi.resetAllMocks());

describe("SMTP settings", () => {
  it("never echoes the stored password and keeps it when the field stays empty", async () => {
    const wrapper = await render();
    const password = input(wrapper, "密码 / 授权码");
    expect((password.element as HTMLInputElement).value).toBe("");
    expect(password.attributes("placeholder")).toBe("已保存，留空不修改");
    await input(wrapper, "SMTP 服务器").setValue("smtp.other.test");
    apiMock.saveMailSettings.mockResolvedValue({ ...saved, host: "smtp.other.test" });
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(apiMock.saveMailSettings).toHaveBeenCalledWith({
      enabled: true,
      host: "smtp.other.test",
      port: 465,
      encryption: "tls",
      username: "noreply@example.test",
      from_address: "noreply@example.test",
      from_name: "ImgNest",
    });
  });

  it("sends a new password once and clears the field after saving", async () => {
    const wrapper = await render();
    await input(wrapper, "密码 / 授权码").setValue("app-password");
    apiMock.saveMailSettings.mockResolvedValue(saved);
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(apiMock.saveMailSettings.mock.calls[0]?.[0].password).toBe("app-password");
    expect((input(wrapper, "密码 / 授权码").element as HTMLInputElement).value).toBe("");
    expect(wrapper.text()).not.toContain("app-password");
  });

  it("switches to the default port of the chosen encryption", async () => {
    const wrapper = await render();
    wrapper.findComponent(NSelect).vm.$emit("update:value", "starttls");
    await flushPromises();
    apiMock.saveMailSettings.mockResolvedValue(saved);
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(apiMock.saveMailSettings.mock.calls[0]?.[0]).toMatchObject({
      encryption: "starttls",
      port: 587,
    });
  });

  it("explains why a test email failed", async () => {
    const wrapper = await render();
    apiMock.sendTestMail.mockRejectedValue(
      new ApiError(50006, "mail failed", 502, { reason: "auth" }),
    );
    await wrapper.get('button[data-action="test-mail"]').trigger("click");
    await flushPromises();
    expect(apiMock.sendTestMail).toHaveBeenCalledWith("");
    expect(wrapper.get("[data-mail-test-error]").text()).toContain("用户名或密码（授权码）被拒绝");
  });

  it("disables the test while mail is off", async () => {
    const wrapper = await render({ ...saved, enabled: false });
    expect(wrapper.get('button[data-action="test-mail"]').attributes("disabled")).toBeDefined();
  });
});
