import { flushPromises, mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { defineComponent, h } from "vue";
import { afterEach, expect, it, vi } from "vitest";
import ClassicLayout from "./ClassicLayout.vue";
import { shellTestRouter } from "./marvis/testing";

vi.mock("@/api/site", () => ({
  fetchSite: vi
    .fn()
    .mockResolvedValue({ site_name: "ImageNest", register_enabled: false, gallery_enabled: false }),
}));

// 只关心经典布局往 Vben 头部插了什么：用一个渲染全部具名插槽的替身代替 BasicLayout。
const BasicLayout = defineComponent({
  setup(_, { slots }) {
    return () =>
      h(
        "div",
        Object.values(slots).map((slot) => slot?.()),
      );
  },
});
const SettingsModal = defineComponent({
  props: { show: Boolean },
  setup: (props) => () => h("div", { "data-testid": "settings-modal", "data-show": props.show }),
});

afterEach(() => {
  document.body.innerHTML = "";
});

it("opens the same settings dialog as the new layout from the header", async () => {
  const router = shellTestRouter();
  await router.push("/upload");
  const wrapper = mount(ClassicLayout, {
    global: {
      plugins: [createPinia(), router],
      stubs: { BasicLayout, MarvisSettingsModal: SettingsModal },
    },
  });
  await flushPromises();
  const modal = () => wrapper.find("[data-testid='settings-modal']");
  expect(modal().attributes("data-show")).toBe("false");
  await wrapper.find("[data-testid='classic-settings-button']").trigger("click");
  expect(modal().attributes("data-show")).toBe("true");
});
