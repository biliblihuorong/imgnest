import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { NMessageProvider } from "naive-ui";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ImageView } from "@/api/upload";
import UploadResultActions from "./UploadResultActions.vue";
import { buildLinkText } from "./linkText";

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

function mountActions(image: ImageView) {
  const host = defineComponent({
    render: () => h(NMessageProvider, () => [h(UploadResultActions, { image })]),
  });
  return mount(host);
}

/** 按 NRadio 文本找单选框并选中（native input，setValue 触发 change）。 */
async function selectRadio(actions: ReturnType<typeof mountActions>, label: string): Promise<void> {
  const radio = actions.findAll(".n-radio").find((node) => node.text().trim() === label);
  if (!radio) {
    throw new Error(`radio ${label} not found`);
  }
  await radio.find("input").setValue(true);
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

describe("UploadResultActions", () => {
  let writeText: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal("navigator", { userAgent: "vitest", clipboard: { writeText } });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  async function copy(actions: ReturnType<typeof mountActions>): Promise<void> {
    const button = actions.findAll("button").find((node) => node.text() === "复制");
    if (!button) {
      throw new Error("copy button not found");
    }
    await button.trigger("click");
  }

  it("默认复制原图 URL", async () => {
    const image = makeImage();
    const actions = mountActions(image);

    await copy(actions);

    expect(writeText).toHaveBeenCalledWith(image.links.original);
  });

  it("切换 WebP 后复制 WebP 链接", async () => {
    const image = makeImage();
    const actions = mountActions(image);

    await selectRadio(actions, "WebP");
    await copy(actions);

    expect(writeText).toHaveBeenCalledWith(image.links.webp);
  });

  it("切换 Markdown 后拼接 ![name](link)", async () => {
    const image = makeImage();
    const actions = mountActions(image);

    await selectRadio(actions, "Markdown");
    await copy(actions);

    expect(writeText).toHaveBeenCalledWith(`![a.png](${image.links.original})`);
  });

  it("切换 HTML / BBCode 后拼接对应格式", async () => {
    const image = makeImage();
    const actions = mountActions(image);

    await selectRadio(actions, "HTML");
    await copy(actions);
    expect(writeText).toHaveBeenLastCalledWith(`<img src="${image.links.original}" alt="a.png" />`);

    await selectRadio(actions, "BBCode");
    await copy(actions);
    expect(writeText).toHaveBeenLastCalledWith(`[img]${image.links.original}[/img]`);
  });

  it("links.webp 为空时 WebP 选项禁用，缺省仍是原图", async () => {
    const image = makeImage({ webp: "", thumbnail_url: "" });
    const actions = mountActions(image);

    await copy(actions);
    expect(writeText).toHaveBeenCalledWith(image.links.original);

    const webpRadio = actions.findAll(".n-radio").find((node) => node.text().trim() === "WebP");
    expect(webpRadio).toBeDefined();
    expect((webpRadio?.find("input").element as HTMLInputElement).disabled).toBe(true);
  });

  it("原图为空时缺省选 WebP", async () => {
    const image = makeImage({ original: "" });
    const actions = mountActions(image);

    await copy(actions);

    expect(writeText).toHaveBeenCalledWith(image.links.webp);
  });

  it("选中版本缺失时回退到 links.url", async () => {
    const image = makeImage({ original: "", webp: "", thumbnail_url: "" });
    const actions = mountActions(image);

    await copy(actions);

    expect(writeText).toHaveBeenCalledWith(image.links.url);
  });

  it("剪贴板写入失败时不抛出", async () => {
    writeText.mockRejectedValue(new Error("denied"));
    const actions = mountActions(makeImage());

    await copy(actions);

    expect(writeText).toHaveBeenCalledTimes(1);
  });
});

it("does not send repeated clipboard writes while copying", async () => {
  const writeText = vi.fn().mockReturnValue(new Promise(() => {}));
  vi.stubGlobal("navigator", { userAgent: "vitest", clipboard: { writeText } });
  const wrapper = mountActions(makeImage());
  const button = wrapper.find("button");
  await button.trigger("click");
  await button.trigger("click");
  await flushPromises();
  expect(writeText).toHaveBeenCalledTimes(1);
  wrapper.unmount();
  vi.unstubAllGlobals();
});
it("keeps each upload result radio group independent", () => {
  const wrapper = mount(() =>
    h(NMessageProvider, () => [
      h(UploadResultActions, { image: makeImage() }),
      h(UploadResultActions, { image: makeImage() }),
    ]),
  );
  const [first, second] = wrapper.findAllComponents(UploadResultActions);
  expect(first!.find("input[type=radio]").attributes("name")).not.toBe(
    second!.find("input[type=radio]").attributes("name"),
  );
  wrapper.unmount();
});
