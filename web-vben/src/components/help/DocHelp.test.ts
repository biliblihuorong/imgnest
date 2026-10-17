import { i18n } from "@vben/locales";
import { enableAutoUnmount, mount } from "@vue/test-utils";
import naive from "naive-ui";
import { afterEach, describe, expect, it } from "vitest";
import { nextTick } from "vue";
import { docUrl } from "@/lib/docs";
import DocHelp from "./DocHelp.vue";

enableAutoUnmount(afterEach);

describe("DocHelp", () => {
  afterEach(() => {
    i18n.global.locale.value = "zh-CN";
  });

  it("在新标签页打开登记表里的文档，并跟随界面语言", async () => {
    i18n.global.locale.value = "zh-CN";
    const wrapper = mount(DocHelp, {
      props: { topic: "randomLinks" },
      global: { plugins: [naive] },
    });
    const link = wrapper.get("a");
    expect(link.attributes("href")).toBe(docUrl("randomLinks", "zh-CN"));
    expect(link.attributes("target")).toBe("_blank");
    expect(link.attributes("rel")).toContain("noopener");
    expect(link.attributes("aria-label")).toBe("查看帮助文档");

    i18n.global.locale.value = "en-US";
    await nextTick();
    expect(link.attributes("href")).toBe(docUrl("randomLinks", "en-US"));
  });
});
