import { enableAutoUnmount, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it } from "vitest";
import { makeAlbum } from "./fixtures";
import AlbumCard from "./AlbumCard.vue";

enableAutoUnmount(afterEach);

function mountCard(album = makeAlbum()) {
  return mount(AlbumCard, { props: { album } });
}

describe("AlbumCard", () => {
  it("渲染名称、图片数与公开标记", () => {
    const wrapper = mountCard(makeAlbum({ name: "旅行", image_count: 5, is_public: true }));

    expect(wrapper.text()).toContain("旅行");
    expect(wrapper.text()).toContain("5 张图片");
    expect(wrapper.text()).toContain("公开");
    expect(wrapper.text()).toContain("2026 年的旅途记录");
  });

  it("is_public=false 显示私有标记", () => {
    const wrapper = mountCard(makeAlbum({ is_public: false }));

    expect(wrapper.text()).toContain("私有");
  });

  it("cover_thumb_url 为空串时显示无封面占位", () => {
    const wrapper = mountCard(makeAlbum({ cover_thumb_url: "" }));

    expect(wrapper.find("img").exists()).toBe(false);
    expect(wrapper.text()).toContain("无封面");
  });

  it("封面加载失败后切换为占位", async () => {
    const wrapper = mountCard(makeAlbum({ cover_thumb_url: "/t/cover.webp" }));

    expect(wrapper.find("img").exists()).toBe(true);
    await wrapper.find("img").trigger("error");

    expect(wrapper.find("img").exists()).toBe(false);
    expect(wrapper.text()).toContain("无封面");
  });

  it("点击编辑发出 edit", async () => {
    const wrapper = mountCard();

    await [...wrapper.findAll("button")].find((b) => b.text() === "编辑")!.trigger("click");

    expect(wrapper.emitted("edit")).toHaveLength(1);
  });

  it("删除需经过 Popconfirm 确认（文案注明图片保留）后才发出 remove", async () => {
    const wrapper = mountCard();

    await [...wrapper.findAll("button")].find((b) => b.text() === "删除")!.trigger("click");
    expect(wrapper.emitted("remove")).toBeUndefined();

    const panel = document.body.querySelector(".n-popconfirm__panel");
    expect(panel?.textContent).toContain("图片会保留");

    const confirm = [...document.body.querySelectorAll("button")].find(
      (button) => button.textContent?.trim() === "确认删除",
    );
    expect(confirm).toBeDefined();
    confirm!.click();
    await Promise.resolve();

    expect(wrapper.emitted("remove")).toHaveLength(1);
  });
});
