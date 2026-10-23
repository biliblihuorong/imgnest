import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { NMessageProvider } from "naive-ui";
import { defineComponent, h } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as api from "@/api/plugins";
import UserPluginCard from "./UserPluginCard.vue";

vi.mock("@/api/plugins", async (original) => ({
  ...(await original<typeof api>()),
  saveUserPluginSettings: vi.fn(),
  putUserPluginAsset: vi.fn(),
  deleteUserPluginAsset: vi.fn(),
  getUserPluginAsset: vi.fn(),
}));
enableAutoUnmount(afterEach);
const card = {
  name: "example",
  title: "My watermark",
  fields: [{ key: "text", label: "Text", type: "text" as const }],
  values: { text: "Alice" },
  read_only: false,
  has_asset: false,
  asset_label: "Logo",
};
function render(plugin = card) {
  return mount(
    defineComponent({ render: () => h(NMessageProvider, () => h(UserPluginCard, { plugin })) }),
  );
}
describe("personal extension card", () => {
  it("saves a private draft and prevents concurrent edits", async () => {
    let resolve!: (value: typeof card) => void;
    vi.mocked(api.saveUserPluginSettings).mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
    const wrapper = render();
    await wrapper.get('input[aria-label="Text"]').setValue("My text");
    await wrapper.get('[data-action="save-user-plugin"]').trigger("click");
    expect(api.saveUserPluginSettings).toHaveBeenCalledWith(
      "example",
      { text: "My text" },
      expect.any(AbortSignal),
    );
    expect(wrapper.get('input[aria-label="Text"]').attributes("disabled")).toBeDefined();
    resolve({ ...card, values: { text: "My text" } });
    await flushPromises();
    expect(card.values.text).toBe("Alice");
  });
  it("shows group policy and disables all personal mutations", () => {
    const wrapper = render({
      ...card,
      read_only: true,
      notice: "Controlled by group",
    } as typeof card);
    expect(wrapper.text()).toContain("Controlled by group");
    expect(wrapper.get('input[aria-label="Text"]').attributes("disabled")).toBeDefined();
    expect(wrapper.find('input[type="file"]').exists()).toBe(false);
    expect(wrapper.find('[data-action="save-user-plugin"]').exists()).toBe(false);
  });
});
