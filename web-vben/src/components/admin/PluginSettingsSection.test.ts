import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { NCheckbox, NDynamicTags, NInput, NMessageProvider, NSelect, NSwitch } from "naive-ui";
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

const groupFixture: adminApi.GroupView = {
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
};

const policyFixture: adminApi.PolicyView = {
  id: 1,
  name: "默认规则",
  storage_id: 3,
  enabled: true,
  path_tpl: "{Y}/{m}/{d}",
  name_tpl: "{uniqid}",
  webp_mode: "both",
  responsive_variants: false,
  webp_quality: 80,
  webp_lossless: false,
  webp_effort: 4,
  max_width: 0,
  max_height: 0,
  thumb_enabled: true,
  thumb_size: 400,
  scrub_mode: "gps",
  heif_mode: "webp_only",
  link_prefer: "webp",
  on_conflict: "rename",
  strip_meta: true,
  skip_if_larger: true,
  created_at: "2026-10-13T00:00:00Z",
  updated_at: "2026-10-13T00:00:00Z",
};

const storageFixture: adminApi.StorageView = {
  id: 3,
  name: "本机存储",
  driver: "local",
  base_url: "http://localhost:8080/i/3",
  path_prefix: "",
  enabled: true,
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((finish) => {
    resolve = finish;
  });
  return { promise, resolve };
}

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
  // Message lifetime is unrelated to these settings assertions. Avoid leaving
  // the library's auto-hide timer running after the test DOM is torn down.
  const wrapper = mount(() => h(NMessageProvider, { duration: 0 }, () => h(PluginSettingsSection)));
  await flushPromises();
  return wrapper;
}

beforeEach(() => {
  vi.resetAllMocks();
  adminMock.listGroups.mockResolvedValue([groupFixture]);
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

  it.each([
    { completionOrder: ["groups", "policies", "storages"] as const },
    { completionOrder: ["storages", "groups", "policies"] as const },
  ])("retains all source options after $completionOrder finish", async ({ completionOrder }) => {
    const groups = deferred<adminApi.GroupView[]>();
    const policies = deferred<adminApi.PolicyView[]>();
    const storages = deferred<adminApi.StorageView[]>();
    adminMock.listGroups.mockReturnValue(groups.promise);
    adminMock.listPolicies.mockReturnValue(policies.promise);
    adminMock.listStorages.mockReturnValue(storages.promise);
    pluginsMock.listPluginSettings.mockResolvedValue([
      webhookCard({
        fields: [
          { key: "group", label: "用户组", type: "select", options_from: "groups" },
          { key: "policy", label: "规则", type: "select", options_from: "policies" },
          {
            key: "targets",
            label: "目标",
            type: "list",
            fields: [{ key: "storage", label: "存储", type: "select", options_from: "storages" }],
          },
        ],
        values: {
          group: null,
          policy: null,
          targets: [{ _key: "0123456789abcdef", storage: null }],
        },
      }),
    ]);
    const wrapper = await mountSection();
    const finish = {
      groups: () => groups.resolve([groupFixture]),
      policies: () => policies.resolve([policyFixture]),
      storages: () => storages.resolve([storageFixture]),
    };
    for (const source of completionOrder) {
      finish[source]();
      await flushPromises();
    }

    const selects = wrapper.findAllComponents(NSelect);
    expect(selects.map((select) => select.props("options"))).toEqual([
      [{ label: "VIP", value: 2 }],
      [{ label: "默认规则", value: 1 }],
      [{ label: "本机存储", value: 3 }],
    ]);
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

  it("preserves unsaved fields and a replacement secret after a refused save", async () => {
    pluginsMock.listPluginSettings.mockResolvedValue([webhookCard()]);
    pluginsMock.savePluginSettings
      .mockRejectedValueOnce(new ApiError(10005, "地址暂不可用", 400))
      .mockResolvedValueOnce(webhookCard());
    const wrapper = await mountSection();
    await wrapper.get('input[aria-label="地址"]').setValue("https://h/retry");
    await wrapper.get('input[aria-label="令牌"]').setValue("replacement-secret");
    await wrapper.find('[data-action="save"]').trigger("click");
    await flushPromises();

    expect((wrapper.get('input[aria-label="地址"]').element as HTMLInputElement).value).toBe(
      "https://h/retry",
    );
    expect((wrapper.get('input[aria-label="令牌"]').element as HTMLInputElement).value).toBe(
      "replacement-secret",
    );
    expect((wrapper.get('input[aria-label="令牌"]').element as HTMLInputElement).disabled).toBe(
      false,
    );
    await wrapper.find('[data-action="save"]').trigger("click");
    await flushPromises();
    expect(pluginsMock.savePluginSettings).toHaveBeenLastCalledWith("webhook", {
      enabled: false,
      token: "replacement-secret",
      hosts: ["a.com"],
      groups: [],
      targets: [
        {
          _key: "0123456789abcdef",
          name: "运维群",
          url: "https://h/retry",
          secret: pluginsApi.SECRET_KEPT,
        },
      ],
    });
  });

  it("requires an explicit clear action for saved top-level and list secrets", async () => {
    pluginsMock.listPluginSettings.mockResolvedValue([webhookCard()]);
    pluginsMock.savePluginSettings.mockResolvedValue(webhookCard());
    const wrapper = await mountSection();
    await wrapper.get('input[aria-label="令牌"]').setValue("replacement-secret");
    await wrapper.get('input[aria-label="令牌"]').setValue("");
    await wrapper.find('[data-action="save"]').trigger("click");
    await flushPromises();
    expect(pluginsMock.savePluginSettings).toHaveBeenLastCalledWith(
      "webhook",
      expect.objectContaining({ token: pluginsApi.SECRET_KEPT }),
    );

    const clear = wrapper.findAllComponents(NCheckbox);
    await clear[0]!.trigger("click");
    await clear[1]!.trigger("click");
    expect((wrapper.get('input[aria-label="令牌"]').element as HTMLInputElement).disabled).toBe(
      true,
    );
    expect((wrapper.get('input[aria-label="密钥"]').element as HTMLInputElement).disabled).toBe(
      true,
    );
    await wrapper.find('[data-action="save"]').trigger("click");
    await flushPromises();
    expect(pluginsMock.savePluginSettings).toHaveBeenLastCalledWith(
      "webhook",
      expect.objectContaining({
        token: "",
        targets: [
          {
            _key: "0123456789abcdef",
            name: "运维群",
            url: "https://h/1",
            secret: "",
          },
        ],
      }),
    );
  });

  it("replaces a newly saved password with the keep/clear controls for future saves", async () => {
    pluginsMock.listPluginSettings.mockResolvedValue([
      webhookCard({ values: { ...webhookCard().values, token: "" } }),
    ]);
    pluginsMock.savePluginSettings.mockResolvedValue(webhookCard());
    const wrapper = await mountSection();
    expect(wrapper.findAllComponents(NCheckbox)).toHaveLength(1);
    await wrapper.get('input[aria-label="令牌"]').setValue("new-secret");
    await wrapper.find('[data-action="save"]').trigger("click");
    await flushPromises();

    expect((wrapper.get('input[aria-label="令牌"]').element as HTMLInputElement).value).toBe("");
    expect(wrapper.get('input[aria-label="令牌"]').attributes("placeholder")).toBe(
      "已设置，留空保持不变",
    );
    expect(wrapper.findAllComponents(NCheckbox)).toHaveLength(2);
    await wrapper.find('[data-action="save"]').trigger("click");
    await flushPromises();
    expect(pluginsMock.savePluginSettings).toHaveBeenLastCalledWith(
      "webhook",
      expect.objectContaining({ token: pluginsApi.SECRET_KEPT }),
    );
  });

  it("warns when secrets cannot be saved", async () => {
    pluginsMock.listPluginSettings.mockResolvedValue([webhookCard({ secrets_available: false })]);
    const wrapper = await mountSection();
    expect(wrapper.text()).toContain("security.master_key");
  });
});
