import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { NDynamicTags, NInput, NMessageProvider, NSelect, NSwitch } from "naive-ui";
import { h } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as adminApi from "@/api/admin";
import * as pluginsApi from "@/api/plugins";
import type { PluginSettings } from "@/api/plugins";
import { ApiError } from "@/api/client";
import PluginSettingsSection from "./PluginSettingsSection.vue";

vi.mock("@/api/admin", () => ({
  listGroups: vi.fn(),
  listPolicies: vi.fn(),
  listStorages: vi.fn(),
}));

vi.mock("@/api/plugins", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/plugins")>()),
  listPluginSettings: vi.fn(),
  savePluginSettings: vi.fn(),
}));

class ResizeObserverStub {
  observe = vi.fn();
  unobserve = vi.fn();
  disconnect = vi.fn();
}
vi.stubGlobal("ResizeObserver", ResizeObserverStub);

enableAutoUnmount(afterEach);

const adminMock = vi.mocked(adminApi);
const pluginsMock = vi.mocked(pluginsApi);

function webhookCard(overrides: Partial<PluginSettings> = {}): PluginSettings {
  return {
    name: "webhook",
    title: "Webhook 通知",
    description: "把事件推送到外部地址",
    secrets_available: true,
    status: [{ label: "授权", value: "有效", level: "success" }],
    fields: [
      { key: "enabled", label: "启用", type: "bool" },
      { key: "token", label: "令牌", type: "secret" },
      { key: "hosts", label: "域名", type: "tags" },
      { key: "groups", label: "用户组", type: "select", options_from: "groups", multiple: true },
      {
        key: "targets",
        label: "目标",
        type: "list",
        item_label: "name",
        fields: [
          { key: "name", label: "名称", type: "text" },
          { key: "url", label: "地址", type: "text", required: true },
          { key: "secret", label: "密钥", type: "secret" },
        ],
      },
    ],
    values: {
      enabled: false,
      token: pluginsApi.SECRET_KEPT,
      hosts: ["a.com"],
      groups: [],
      targets: [
        {
          _key: "0123456789abcdef",
          name: "运维群",
          url: "https://h/1",
          secret: pluginsApi.SECRET_KEPT,
        },
      ],
    },
    ...overrides,
  };
}

async function mountSection(): Promise<VueWrapper> {
  const wrapper = mount(() => h(NMessageProvider, () => h(PluginSettingsSection)));
  await flushPromises();
  return wrapper;
}

beforeEach(() => {
  vi.resetAllMocks();
  adminMock.listGroups.mockResolvedValue([
    {
      id: 2,
      name: "VIP",
      is_default: false,
      is_guest: false,
      capacity_bytes: 0,
      max_file_bytes: 0,
      allowed_exts: [],
      upload_per_min: 0,
      default_policy_id: 1,
      policy_ids: [],
      user_count: 0,
    },
  ]);
});

describe("PluginSettingsSection", () => {
  it("renders nothing when the server has no plugin cards", async () => {
    pluginsMock.listPluginSettings.mockResolvedValue([]);
    const wrapper = await mountSection();
    expect(wrapper.find("[data-plugin-settings]").exists()).toBe(false);
    expect(adminMock.listGroups).not.toHaveBeenCalled();
  });

  it("renders a card with status, fields, list items and source options", async () => {
    pluginsMock.listPluginSettings.mockResolvedValue([webhookCard()]);
    const wrapper = await mountSection();
    const card = wrapper.find('[data-plugin="webhook"]');
    expect(card.text()).toContain("Webhook 通知");
    expect(card.find("[data-plugin-status]").text()).toContain("有效");
    expect(card.findAll("[data-list-item]")).toHaveLength(1);
    expect(card.text()).toContain("运维群");
    expect(adminMock.listGroups).toHaveBeenCalledOnce();
    const select = wrapper.findComponent(NSelect);
    expect(select.props("options")).toEqual([{ label: "VIP", value: 2 }]);
    expect(wrapper.findComponent(NDynamicTags).props("value")).toEqual(["a.com"]);
  });

  it("keeps saved secrets unless they are retyped, and sends new list items without a key", async () => {
    pluginsMock.listPluginSettings.mockResolvedValue([webhookCard()]);
    pluginsMock.savePluginSettings.mockResolvedValue(
      webhookCard({ values: { ...webhookCard().values, enabled: true } }),
    );
    const wrapper = await mountSection();
    await wrapper.findComponent(NSwitch).vm.$emit("update:value", true);
    await wrapper.find('[data-action="add-item"]').trigger("click");
    const inputs = wrapper.findAllComponents(NInput);
    // inputs: token, item1 name/url/secret, item2 name/url/secret
    expect(inputs[0].props("value")).toBe("");
    await inputs[5].vm.$emit("update:value", "https://h/2");
    await inputs[6].vm.$emit("update:value", "new-secret");
    await wrapper.find('[data-action="save"]').trigger("click");
    await flushPromises();
    expect(pluginsMock.savePluginSettings).toHaveBeenCalledWith("webhook", {
      enabled: true,
      token: pluginsApi.SECRET_KEPT,
      hosts: ["a.com"],
      groups: [],
      targets: [
        {
          _key: "0123456789abcdef",
          name: "运维群",
          url: "https://h/1",
          secret: pluginsApi.SECRET_KEPT,
        },
        { name: "", url: "https://h/2", secret: "new-secret" },
      ],
    });
  });

  it("shows the plugin's own message for a 10005 refusal", async () => {
    pluginsMock.listPluginSettings.mockResolvedValue([webhookCard()]);
    pluginsMock.savePluginSettings.mockRejectedValue(
      new ApiError(10005, "目标地址必须是 https", 400),
    );
    const wrapper = await mountSection();
    await wrapper.find('[data-action="save"]').trigger("click");
    await flushPromises();
    expect(document.body.textContent).toContain("目标地址必须是 https");
  });

  it("warns when secrets cannot be saved", async () => {
    pluginsMock.listPluginSettings.mockResolvedValue([webhookCard({ secrets_available: false })]);
    const wrapper = await mountSection();
    expect(wrapper.text()).toContain("security.master_key");
  });
});
