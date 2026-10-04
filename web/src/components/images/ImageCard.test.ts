import { enableAutoUnmount, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it } from "vitest";
import type { ImageView } from "@/api/images";
import { makeImage } from "./fixtures";
import ImageCard from "./ImageCard.vue";

enableAutoUnmount(afterEach);

function mountCard(image: ImageView = makeImage()) {
  return mount(ImageCard, { props: { image } });
}

describe("ImageCard", () => {
  it("渲染名称、formatBytes 大小与私有标记", () => {
    const wrapper = mountCard(makeImage({ name: "photo.png", size: 2048, is_public: false }));

    expect(wrapper.text()).toContain("photo.png");
    expect(wrapper.text()).toContain("2.0 KB");
    expect(wrapper.text()).toContain("100×80");
    expect(wrapper.text()).toContain("私有");
  });

  it("is_public=true 显示公开标记", () => {
    const wrapper = mountCard(makeImage({ is_public: true }));

    expect(wrapper.text()).toContain("公开");
  });

  it("local_thumb_url 缺失时显示 ext 占位", () => {
    const wrapper = mountCard(makeImage({ local_thumb_url: "" }));

    expect(wrapper.find("img").exists()).toBe(false);
    expect(wrapper.text()).toContain("PNG");
    expect(wrapper.text()).toContain("无缩略图");
  });

  it("缩略图加载失败后切换为占位", async () => {
    const wrapper = mountCard();

    expect(wrapper.find("img").exists()).toBe(true);
    await wrapper.find("img").trigger("error");

    expect(wrapper.find("img").exists()).toBe(false);
    expect(wrapper.text()).toContain("无缩略图");
  });

  it("点击缩略图与名称触发 open", async () => {
    const wrapper = mountCard();

    await wrapper.find(".image-card__thumb").trigger("click");
    await wrapper.find(".image-card__name").trigger("click");

    expect(wrapper.emitted("open")).toHaveLength(2);
  });

  it("点击开关发出 toggle，受控值保持不变（回滚由父级处理）", async () => {
    const wrapper = mountCard(makeImage({ is_public: false }));
    const sw = wrapper.find(".n-switch");

    await sw.trigger("click");

    expect(wrapper.emitted("toggle")).toEqual([[true]]);
    expect(sw.attributes("aria-checked")).toBe("false");
  });

  it("删除需经过 Popconfirm 确认后才发出 remove", async () => {
    const wrapper = mountCard();

    await wrapper.find("button").trigger("click");
    expect(wrapper.emitted("remove")).toBeUndefined();

    const confirmButton = [...document.body.querySelectorAll("button")].find(
      (button) => button.textContent?.trim() === "确认删除",
    );
    expect(confirmButton).toBeDefined();
    confirmButton!.click();
    await Promise.resolve();

    expect(wrapper.emitted("remove")).toHaveLength(1);
  });
});
