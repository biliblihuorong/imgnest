import { enableAutoUnmount, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it } from "vitest";
import { NImage } from "naive-ui";
import type { GalleryItem } from "@/api/gallery";
import type { ImageView } from "@/api/images";
import { makeImage } from "@/components/images/fixtures";
import GalleryCard from "./GalleryCard.vue";

enableAutoUnmount(afterEach);

function makeItem(image: Partial<ImageView> = {}, uploader?: string): GalleryItem {
  const base = makeImage(image);
  return { ...base, uploader: uploader ?? "" };
}

function mountCard(item: GalleryItem) {
  return mount(GalleryCard, { props: { item } });
}

describe("GalleryCard", () => {
  it("有云缩略图时 src 用 thumbnail_url，预览用原图直链并开启懒加载", () => {
    const wrapper = mountCard(
      makeItem(
        {
          name: "photo.png",
          links: {
            url: "/i/1/k.png",
            original: "/i/1/k.png",
            webp: "",
            thumbnail_url: "/t/1/k.webp",
          },
        },
        "alice",
      ),
    );

    expect(wrapper.find("img").attributes("src")).toBe("/t/1/k.webp");
    const nimage = wrapper.findComponent(NImage);
    expect(nimage.props("previewSrc")).toBe("/i/1/k.png");
    expect(nimage.props("lazy")).toBe(true);
    expect(wrapper.find("img").attributes("alt")).toBe("photo.png");
  });

  it("缩略图缺失（空串）时回退 links.url", () => {
    const wrapper = mountCard(
      makeItem({
        links: { url: "/i/2/k.jpg", original: "/i/2/k.jpg", webp: "", thumbnail_url: "" },
      }),
    );

    expect(wrapper.find("img").attributes("src")).toBe("/i/2/k.jpg");
  });

  it("渲染名称与上传者用户名", () => {
    const wrapper = mountCard(makeItem({ name: "sunset.jpg" }, "bob"));

    expect(wrapper.text()).toContain("sunset.jpg");
    expect(wrapper.find(".gallery-card__uploader").text()).toBe("bob");
  });

  it("无 uploader 时不渲染上传者节点", () => {
    const wrapper = mountCard(makeItem({ name: "solo.png" }));

    expect(wrapper.find(".gallery-card__uploader").exists()).toBe(false);
  });
});
