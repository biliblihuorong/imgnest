import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createPinia, setActivePinia, type Pinia } from "pinia";
import { NAlert, NImage, NPagination } from "naive-ui";
import { nextTick } from "vue";
import { i18n } from "@vben/locales";
import { listGallery, type GalleryItem, type GalleryPage } from "@/api/gallery";
import GalleryCard from "@/components/gallery/GalleryCard.vue";
import { makeImage } from "@/components/images/fixtures";
import { useSiteStore } from "@/stores/site";
import GalleryView from "./GalleryView.vue";

vi.mock("@/api/gallery", () => ({
  listGallery: vi.fn(),
}));

const listGalleryMock = vi.mocked(listGallery);

enableAutoUnmount(afterEach);

let pinia: Pinia;

function makeItem(overrides: Parameters<typeof makeImage>[0] = {}, uploader?: string): GalleryItem {
  const base = makeImage(overrides);
  return { ...base, uploader: uploader ?? "" };
}

function makePage(overrides: Partial<GalleryPage> = {}): GalleryPage {
  return { items: [], total: 0, page: 1, size: 30, ...overrides };
}

function mountGallery() {
  return mount(GalleryView, { global: { plugins: [pinia] } });
}

beforeEach(() => {
  i18n.global.locale.value = "zh-CN";
  vi.resetAllMocks();
  pinia = createPinia();
  setActivePinia(pinia);
  // 预置站点状态并置 loaded，避免视图里的 ensureLoaded 触发真实请求。
  const site = useSiteStore();
  site.loaded = true;
  site.galleryEnabled = true;
  listGalleryMock.mockResolvedValue(makePage());
});

describe("GalleryView", () => {
  it("挂载后按 page=1&size=30 加载并渲染瀑布流卡片", async () => {
    listGalleryMock.mockResolvedValue(
      makePage({
        items: [
          makeItem(
            {
              id: 1,
              name: "a.png",
              links: {
                url: "/i/a.png",
                original: "/i/a.png",
                webp: "",
                thumbnail_url: "/t/a.webp",
              },
            },
            "alice",
          ),
          makeItem({
            id: 2,
            name: "b.jpg",
            links: { url: "/i/b.jpg", original: "/i/b.jpg", webp: "", thumbnail_url: "" },
          }),
        ],
        total: 42,
      }),
    );
    const wrapper = mountGallery();
    await flushPromises();

    expect(listGalleryMock).toHaveBeenCalledWith({ page: 1, size: 30 });
    expect(wrapper.findAllComponents(GalleryCard)).toHaveLength(2);
    expect(wrapper.text()).toContain("a.png");
    expect(wrapper.text()).toContain("b.jpg");
    expect(wrapper.text()).toContain("alice");
    expect(wrapper.text()).toContain("共 42 张公开图片");
  });

  it("缩略图优先、缺失回退原图直链，预览始终用原图", async () => {
    listGalleryMock.mockResolvedValue(
      makePage({
        items: [
          makeItem({
            id: 1,
            links: { url: "/i/a.png", original: "/i/a.png", webp: "", thumbnail_url: "/t/a.webp" },
          }),
          makeItem({
            id: 2,
            links: { url: "/i/b.jpg", original: "/i/b.jpg", webp: "", thumbnail_url: "" },
          }),
        ],
      }),
    );
    const wrapper = mountGallery();
    await flushPromises();

    const images = wrapper.findAll("img");
    expect(images[0].attributes("src")).toBe("/t/a.webp");
    expect(images[1].attributes("src")).toBe("/i/b.jpg");

    const nimages = wrapper
      .findAllComponents(GalleryCard)
      .map((card) => card.findComponent(NImage));
    expect(nimages[0].props("previewSrc")).toBe("/i/a.png");
    expect(nimages[0].props("lazy")).toBe(true);
  });

  it("uploader 缺失时不展示上传者", async () => {
    listGalleryMock.mockResolvedValue(makePage({ items: [makeItem({ id: 1, name: "solo.png" })] }));
    const wrapper = mountGallery();
    await flushPromises();

    expect(wrapper.find(".gallery-card__uploader").exists()).toBe(false);
  });

  it("翻页时携带新的 page 参数", async () => {
    listGalleryMock.mockResolvedValue(makePage({ total: 42 }));
    const wrapper = mountGallery();
    await flushPromises();

    wrapper.findComponent(NPagination).vm.$emit("update:page", 2);
    await flushPromises();

    expect(listGalleryMock).toHaveBeenLastCalledWith({ page: 2, size: 30 });
  });

  it("galleryEnabled=false 时空态提示站点未开放公开画廊", async () => {
    useSiteStore().galleryEnabled = false;
    listGalleryMock.mockResolvedValue(makePage({ items: [], total: 0 }));
    const wrapper = mountGallery();
    await flushPromises();

    expect(wrapper.text()).toContain("站点未开放公开画廊");
  });

  it("galleryEnabled=true 且无图片时空态提示暂无公开图片", async () => {
    listGalleryMock.mockResolvedValue(makePage({ items: [], total: 0 }));
    const wrapper = mountGallery();
    await flushPromises();

    expect(wrapper.text()).toContain("暂无公开图片");
  });

  it("加载失败显示 NAlert，点击重试重新加载", async () => {
    listGalleryMock.mockRejectedValueOnce(new Error("网络错误"));
    const wrapper = mountGallery();
    await flushPromises();

    expect(wrapper.findComponent(NAlert).exists()).toBe(true);
    expect(wrapper.text()).toContain("画廊加载失败");
    expect(listGalleryMock).toHaveBeenCalledTimes(1);

    listGalleryMock.mockResolvedValue(
      makePage({ items: [makeItem({ id: 9, name: "ok.png" })], total: 1 }),
    );
    const retryButton = wrapper.findAll("button").find((b) => b.text() === "重试");
    expect(retryButton).toBeDefined();
    await retryButton!.trigger("click");
    await flushPromises();

    expect(listGalleryMock).toHaveBeenCalledTimes(2);
    expect(wrapper.findComponent(NAlert).exists()).toBe(false);
    expect(wrapper.text()).toContain("ok.png");
  });

  it("ignores stale gallery pages after a newer page completes", async () => {
    let resolveOlder!: (value: GalleryPage) => void;
    listGalleryMock.mockResolvedValueOnce(makePage({ total: 90 }));
    const wrapper = mountGallery();
    await flushPromises();
    listGalleryMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveOlder = resolve;
        }),
    );
    wrapper.findComponent(NPagination).vm.$emit("update:page", 2);
    await flushPromises();
    listGalleryMock.mockResolvedValueOnce(
      makePage({ items: [makeItem({ name: "Latest photo" })], total: 90, page: 3 }),
    );
    wrapper.findComponent(NPagination).vm.$emit("update:page", 3);
    await flushPromises();
    resolveOlder(makePage({ items: [makeItem({ name: "Stale photo" })], total: 90, page: 2 }));
    await flushPromises();
    expect(wrapper.text()).toContain("Latest photo");
    expect(wrapper.text()).not.toContain("Stale photo");
    expect(wrapper.findComponent(NPagination).props("page")).toBe(3);
  });

  it("hides existing images when the gallery is disabled during a request", async () => {
    let resolvePage!: (value: GalleryPage) => void;
    listGalleryMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolvePage = resolve;
        }),
    );
    const wrapper = mountGallery();
    useSiteStore().galleryEnabled = false;
    await nextTick();
    resolvePage(makePage({ items: [makeItem({ name: "Hidden photo" })], total: 1 }));
    await flushPromises();
    expect(wrapper.findAllComponents(GalleryCard)).toHaveLength(0);
    expect(wrapper.text()).toContain("站点未开放公开画廊");
  });

  it("changes public page and uploader labels without reloading", async () => {
    listGalleryMock.mockResolvedValue(
      makePage({ items: [makeItem({ name: "Public photo" }, "alice")], total: 1 }),
    );
    const wrapper = mountGallery();
    await flushPromises();
    i18n.global.locale.value = "en-US";
    await nextTick();
    expect(wrapper.text()).toContain("Public gallery");
    expect(wrapper.text()).toContain("1 public image");
    expect(wrapper.find(".gallery-card__uploader").attributes("title")).toBe("Uploaded by alice");
    expect(listGalleryMock).toHaveBeenCalledTimes(1);
  });
});
