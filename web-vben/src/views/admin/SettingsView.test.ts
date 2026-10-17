import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { NMessageProvider } from "naive-ui";
import { createPinia, setActivePinia } from "pinia";
import { h } from "vue";
import { createMemoryHistory, createRouter, RouterView } from "vue-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as adminApi from "@/api/admin";
import type { AdminSettings } from "@/api/admin";
import * as mailApi from "@/api/mail";
import * as systemApi from "@/api/system";
import type { SystemVersion } from "@/api/system";
import { settingsRoutes } from "./settings/sections";
import SettingsView from "./SettingsView.vue";

vi.mock("@/components/admin/CaptchaSettingsCard.vue", () => ({
  default: { template: '<section data-testid="captcha-settings" />' },
}));
vi.mock("@/components/admin/MailSettingsCard.vue", () => ({
  default: { template: '<section data-testid="mail-settings" />' },
}));
vi.mock("@/components/admin/PluginSettingsSection.vue", () => ({
  default: { template: '<section data-testid="plugin-settings" />' },
}));
vi.mock("./settings/BackupSettingsView.vue", () => ({
  default: { template: '<section data-testid="backup-settings" />' },
}));
vi.mock("@/api/admin", () => ({ getSettings: vi.fn(), putSettings: vi.fn(), listGroups: vi.fn() }));
vi.mock("@/api/mail", () => ({ getMailSettings: vi.fn() }));
vi.mock("@/api/site", () => ({ fetchSite: vi.fn() }));
vi.mock("@/api/system", () => ({ fetchSystemVersion: vi.fn() }));
vi.stubGlobal(
  "ResizeObserver",
  class {
    observe = vi.fn();
    unobserve = vi.fn();
    disconnect = vi.fn();
  },
);
enableAutoUnmount(afterEach);

const settings: AdminSettings = {
  site_name: "ImgNest",
  registration_enabled: true,
  guest_upload_enabled: false,
  gallery_enabled: true,
  gallery_public_albums_only: false,
  api_enabled: true,
  trash_days: 7,
  guest_group_id: 0,
  default_group_id: 0,
  avatar_provider: "weavatar",
  email_verification_enabled: true,
};

const version: SystemVersion = {
  current: "0.0.9",
  latest: {
    version: "0.1.0",
    url: "https://github.com/biliblihuorong/imgnest/releases/tag/v0.1.0",
    published_at: "2026-10-16T00:00:00Z",
  },
  update_available: false,
  check_enabled: true,
  checked_at: "2026-10-17T00:00:00Z",
  check_failed: false,
  repository_url: "https://github.com/biliblihuorong/imgnest",
  docs_url: "https://github.com/biliblihuorong/imgnest/tree/main/website/docs/guide",
  releases_url: "https://github.com/biliblihuorong/imgnest/releases",
  issues_url: "https://github.com/biliblihuorong/imgnest/issues",
};

async function mountAt(path: string) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      {
        path: "/admin/settings",
        name: "admin-settings",
        component: SettingsView,
        redirect: { name: "admin-settings-general" },
        children: settingsRoutes(),
      },
    ],
  });
  await router.push(path);
  const wrapper = mount(() => h(NMessageProvider, () => h(RouterView)), {
    global: { plugins: [router] },
  });
  await flushPromises();
  return { wrapper, router };
}

beforeEach(() => {
  vi.resetAllMocks();
  setActivePinia(createPinia());
  vi.mocked(adminApi).getSettings.mockResolvedValue({ ...settings });
  vi.mocked(adminApi).listGroups.mockResolvedValue([]);
  vi.mocked(systemApi).fetchSystemVersion.mockResolvedValue({ ...version });
  vi.mocked(mailApi).getMailSettings.mockResolvedValue({
    enabled: false,
    host: "",
    port: 465,
    encryption: "tls",
    username: "",
    password_configured: false,
    from_address: "",
    from_name: "",
    configuration_available: true,
  });
});

describe("SettingsView", () => {
  it("opens the general page and lists every settings page", async () => {
    const { wrapper, router } = await mountAt("/admin/settings");
    expect(router.currentRoute.value.name).toBe("admin-settings-general");
    expect(
      wrapper.findAll(".settings-nav__item").map((item) => item.attributes("data-section")),
    ).toEqual(["general", "auth", "access", "mail", "plugins", "backup", "about"]);
    expect(wrapper.find('[data-testid="settings-update-dot"]').exists()).toBe(false);
    expect(wrapper.find("input[placeholder='站点名称']").exists()).toBe(true);
  });

  it("navigates between pages without reloading the layout", async () => {
    const { wrapper, router } = await mountAt("/admin/settings/general");
    await wrapper.get('[data-section="mail"]').trigger("click");
    await vi.waitFor(() => expect(router.currentRoute.value.path).toBe("/admin/settings/mail"));
    await flushPromises();
    expect(wrapper.find('[data-testid="mail-settings"]').exists()).toBe(true);
    expect(wrapper.get('[data-section="mail"]').classes()).toContain("router-link-active");
  });

  it("warns on the sign-up page when email verification is on but mail is off", async () => {
    const { wrapper } = await mountAt("/admin/settings/auth");
    expect(wrapper.find('[data-testid="email-verification-needs-mail"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="captcha-settings"]').exists()).toBe(true);
  });

  it("shows the plugin page", async () => {
    const { wrapper } = await mountAt("/admin/settings/plugins");
    expect(wrapper.find('[data-testid="plugin-settings"]').exists()).toBe(true);
  });

  it("shows the backup page", async () => {
    const { wrapper, router } = await mountAt("/admin/settings/backup");
    expect(router.currentRoute.value.name).toBe("admin-settings-backup");
    expect(wrapper.find('[data-testid="backup-settings"]').exists()).toBe(true);
  });

  it("shows the version, an update notice and project links on the about page", async () => {
    vi.mocked(systemApi).fetchSystemVersion.mockResolvedValue({
      ...version,
      update_available: true,
    });
    const { wrapper } = await mountAt("/admin/settings/about");
    expect(wrapper.get('[data-testid="about-current"]').text()).toBe("v0.0.9");
    expect(wrapper.get('[data-testid="about-update"]').text()).toContain("v0.1.0");
    expect(wrapper.find('[data-testid="settings-update-dot"]').exists()).toBe(true);
    expect(wrapper.get('[data-link="docs"]').attributes("href")).toBe(version.docs_url);
    expect(wrapper.get('[data-link="repository"]').attributes("rel")).toContain("noopener");
    // Both the settings shell and the page asked, but only one request went out.
    expect(systemApi.fetchSystemVersion).toHaveBeenCalledTimes(1);
  });

  it("checks again on request", async () => {
    const { wrapper } = await mountAt("/admin/settings/about");
    const check = wrapper.findAll("button").find((button) => button.text() === "检查更新");
    await check!.trigger("click");
    await flushPromises();
    expect(systemApi.fetchSystemVersion).toHaveBeenLastCalledWith(true);
  });
});
