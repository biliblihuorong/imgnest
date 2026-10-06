import { flushPromises, mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { defineComponent, h, ref } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";
import { expect, it, vi } from "vitest";
import { i18n, useI18n } from "@vben/locales";
import { updatePreferences } from "@vben/preferences";
import AppLayout from "./ClassicLayout.vue";

vi.mock("@/api/site", () => ({
  fetchSite: vi
    .fn()
    .mockResolvedValue({ site_name: "ImageNest", register_enabled: false, gallery_enabled: false }),
}));

it("the real Vben shell preserves form and queue instances across language switches", async () => {
  updatePreferences({ tabbar: { keepAlive: false }, transition: { enable: false } });
  let mounts = 0;
  const Feature = defineComponent({
    setup() {
      mounts++;
      const draft = ref("");
      const queued = ref(0);
      const { t } = useI18n();
      return () =>
        h("main", [
          h("input", {
            value: draft.value,
            onInput: (event: Event) => (draft.value = (event.target as HTMLInputElement).value),
          }),
          h("button", { onClick: () => queued.value++ }, t("common.save")),
          h("output", String(queued.value)),
        ]);
    },
  });
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/upload", name: "upload", component: Feature, meta: { title: "common.nav.upload" } },
    ],
  });
  await router.push("/upload");
  const wrapper = mount(AppLayout, {
    global: {
      plugins: [createPinia(), router],
      stubs: {
        VbenAdminLayout: { template: '<div><slot name="content" /></div>' },
        VbenLayout: { template: '<div><slot name="content" /></div>' },
        VbenBackTop: true,
        BackTop: true,
      },
    },
  });
  await flushPromises();
  await wrapper.find("input").setValue("Unsubmitted draft");
  await wrapper.find("button").trigger("click");
  for (const language of ["en-US", "zh-CN"]) {
    i18n.global.locale.value = language;
    await flushPromises();
    expect(mounts).toBe(1);
    expect((wrapper.find("input").element as HTMLInputElement).value).toBe("Unsubmitted draft");
    expect(wrapper.find("output").text()).toBe("1");
  }
  wrapper.unmount();
});
