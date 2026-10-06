import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { defineComponent, h, ref } from "vue";
import { NMessageProvider } from "naive-ui";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ImageView } from "@/api/upload";
import UploadResultActions from "./UploadResultActions.vue";
import { buildLinkText, resolveImageLink, type LinkFormat, type LinkVersion } from "./linkText";

function makeImage(partialLinks: Partial<ImageView["links"]> = {}): ImageView {
  return {
    id: 7,
    key: "2026/10/a.png",
    user_id: 1,
    album_id: 0,
    policy_id: 1,
    storage_id: 1,
    name: "a.png",
    ext: "png",
    mime: "image/png",
    size: 1024,
    webp_size: 512,
    charged_bytes: 1536,
    width: 100,
    height: 100,
    frames: 1,
    has_original: true,
    has_webp: true,
    has_thumb: true,
    scrubbed: false,
    is_public: false,
    md5: "m",
    sha1: "s",
    src_md5: "sm",
    links: {
      url: "https://img.example/2026/10/a.png",
      original: "https://img.example/2026/10/a.png",
      webp: "https://img.example/2026/10/a.webp",
      thumbnail_url: "https://img.example/2026/10/a_thumbs.webp",
      ...partialLinks,
    },
    local_thumb_url: "/t/2026/10/a.png",
    deleted_at: null,
    purge_at: null,
    created_at: "2026-10-04T00:00:00Z",
  };
}

interface MountOptions {
  format?: LinkFormat;
  version?: LinkVersion;
}

/** 受控挂载：format/version 由宿主持有，模拟队列头部的全局选择。 */
function mountActions(image: ImageView, options: MountOptions = {}) {
  const format = ref<LinkFormat>(options.format ?? "url");
  const version = ref<LinkVersion>(options.version ?? "webp");
  const host = defineComponent({
    setup() {
      return () =>
        h(NMessageProvider, () => [
          h(UploadResultActions, {
            image,
            format: format.value,
            "onUpdate:format": (value: LinkFormat) => {
              format.value = value;
            },
            version: version.value,
            "onUpdate:version": (value: LinkVersion) => {
              version.value = value;
            },
          }),
        ]);
    },
  });
  const wrapper = mount(host);
  return { wrapper, format, version };
}

describe("buildLinkText", () => {
  it("四种格式的拼接", () => {
    expect(buildLinkText("url", "a.png", "https://x/a.png")).toBe("https://x/a.png");
    expect(buildLinkText("markdown", "a.png", "https://x/a.png")).toBe("![a.png](https://x/a.png)");
    expect(buildLinkText("html", "a.png", "https://x/a.png")).toBe(
      `<img src="https://x/a.png" alt="a.png" />`,
    );
    expect(buildLinkText("bbcode", "a.png", "https://x/a.png")).toBe("[img]https://x/a.png[/img]");
  });
});

describe("resolveImageLink", () => {
  it("按版本取链接，选中版本缺失时回退 links.url", () => {
    const full = makeImage();
    expect(resolveImageLink(full, "original")).toBe(full.links.original);
    expect(resolveImageLink(full, "webp")).toBe(full.links.webp);
    expect(resolveImageLink(full, "thumbnail")).toBe(full.links.thumbnail_url);
    const bare = makeImage({ original: "", webp: "", thumbnail_url: "" });
    expect(resolveImageLink(bare, "webp")).toBe(bare.links.url);
  });
});

describe("UploadResultActions", () => {
  let writeText: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal("navigator", { userAgent: "vitest", clipboard: { writeText } });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  async function copy(wrapper: VueWrapper): Promise<void> {
    const button = wrapper.findAll("button").find((node) => node.text() === "复制");
    if (!button) {
      throw new Error("copy button not found");
    }
    await button.trigger("click");
  }

  it("行内展示文件名与当前全局版本链接（默认 WebP）", () => {
    const image = makeImage();
    const { wrapper } = mountActions(image, { format: "url", version: "webp" });

    expect(wrapper.text()).toContain("a.png");
    expect(wrapper.find(".upload-result__link").text()).toBe(image.links.webp);
  });

  it("复制按钮按当前全局格式/版本复制", async () => {
    const image = makeImage();
    const { wrapper } = mountActions(image, { format: "url", version: "webp" });

    await copy(wrapper);
    expect(writeText).toHaveBeenCalledWith(image.links.webp);

    wrapper.unmount();
    const markdown = mountActions(image, { format: "markdown", version: "original" });
    await copy(markdown.wrapper);
    expect(writeText).toHaveBeenLastCalledWith(`![a.png](${image.links.original})`);
    markdown.wrapper.unmount();
  });

  it("选中版本缺失时链接与复制回退 links.url", async () => {
    const image = makeImage({ original: "", webp: "", thumbnail_url: "" });
    const { wrapper } = mountActions(image, { format: "url", version: "webp" });

    expect(wrapper.find(".upload-result__link").text()).toBe(image.links.url);
    await copy(wrapper);
    expect(writeText).toHaveBeenCalledWith(image.links.url);
  });

  it("点击链接区域或「全部版本」发出 detail 打开抽屉", async () => {
    const image = makeImage();
    const host = defineComponent({
      setup() {
        const format = ref<LinkFormat>("url");
        const version = ref<LinkVersion>("webp");
        return () =>
          h(NMessageProvider, () => [
            h(UploadResultActions, {
              image,
              format: format.value,
              "onUpdate:format": (value: LinkFormat) => {
                format.value = value;
              },
              version: version.value,
              "onUpdate:version": (value: LinkVersion) => {
                version.value = value;
              },
            }),
          ]);
      },
    });
    const wrapper = mount(host);
    const actions = wrapper.findComponent(UploadResultActions);

    await actions.find(".upload-result__main").trigger("click");
    await actions.find("button").trigger("click");

    expect(actions.emitted("detail")).toHaveLength(2);
  });

  it("剪贴板写入失败时不抛出", async () => {
    writeText.mockRejectedValue(new Error("denied"));
    const { wrapper } = mountActions(makeImage());

    await copy(wrapper);

    expect(writeText).toHaveBeenCalledTimes(1);
  });
});

it("does not send repeated clipboard writes while copying", async () => {
  const writeText = vi.fn().mockReturnValue(new Promise(() => {}));
  vi.stubGlobal("navigator", { userAgent: "vitest", clipboard: { writeText } });
  const { wrapper } = mountActions(makeImage());
  const button = [...wrapper.findAll("button")].find((node) => node.text() === "复制")!;
  await button.trigger("click");
  await button.trigger("click");
  await flushPromises();
  expect(writeText).toHaveBeenCalledTimes(1);
  wrapper.unmount();
  vi.unstubAllGlobals();
});
