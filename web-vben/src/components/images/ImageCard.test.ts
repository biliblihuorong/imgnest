import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ImageView } from "@/api/images";
import type { DropdownOption } from "naive-ui";
import { makeImage } from "./fixtures";
import ImageCard from "./ImageCard.vue";

vi.mock("@/api/thumbnails", () => ({
  fetchProtectedThumbnail: vi.fn().mockResolvedValue(new Blob(["image"], { type: "image/webp" })),
}));
beforeEach(() => {
  URL.createObjectURL = vi.fn().mockReturnValue("blob:thumbnail");
  URL.revokeObjectURL = vi.fn();
});
enableAutoUnmount(afterEach);

function mountCard(image: ImageView = makeImage(), props: Record<string, unknown> = {}) {
  return mount(ImageCard, { props: { image, ...props } });
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
    await flushPromises();

    expect(wrapper.find("img").exists()).toBe(true);
    await wrapper.find("img").trigger("error");

    expect(wrapper.find("img").exists()).toBe(false);
    expect(wrapper.text()).toContain("无缩略图");
  });

  it("缩略图 NImage 挂 WebP 大图（由外层分组预览翻页），点名称触发 open", async () => {
    const wrapper = mountCard();
    await flushPromises();

    const image = wrapper.find(".image-card__thumb .n-image");
    expect(image.exists()).toBe(true);
    expect(wrapper.find(".image-card__name").trigger("click"));

    expect(wrapper.emitted("open")).toHaveLength(1);
  });

  it("整卡右键发出 menu 并携带原始事件", async () => {
    const wrapper = mountCard();

    await wrapper.find(".image-card").trigger("contextmenu");

    const menu = wrapper.emitted("menu");
    expect(menu).toHaveLength(1);
    expect(menu?.[0]?.[0]).toBeInstanceOf(MouseEvent);
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

  it("selectable 时显示左上角多选框：点击发出 select 且不触发 open", async () => {
    const wrapper = mountCard(makeImage(), { selectable: true, selected: false });

    expect(wrapper.find(".image-card__check").exists()).toBe(true);
    await wrapper.find(".n-checkbox").trigger("click");

    expect(wrapper.emitted("select")).toEqual([[true]]);
    expect(wrapper.emitted("open")).toBeUndefined();
  });

  it("未启用 selectable 时不显示多选框", () => {
    const wrapper = mountCard();

    expect(wrapper.find(".image-card__check").exists()).toBe(false);
  });

  it("albumOptions 提供时显示移动下拉，选择相册后发出 move", async () => {
    const options: DropdownOption[] = [
      { label: "移出相册", key: 0 },
      { label: "旅行", key: 7 },
    ];
    const wrapper = mountCard(makeImage(), { albumOptions: options });

    await [...wrapper.findAll("button")].find((b) => b.text() === "移动")!.trigger("click");
    await flushPromises();

    const option = [...document.body.querySelectorAll(".n-dropdown-option-body")].find((node) =>
      node.textContent?.includes("旅行"),
    );
    expect(option).toBeDefined();
    option!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    await flushPromises();

    expect(wrapper.emitted("move")).toEqual([[7]]);
  });
});
