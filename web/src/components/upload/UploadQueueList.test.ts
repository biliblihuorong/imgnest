import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { NMessageProvider } from "naive-ui";
import { describe, expect, it } from "vitest";
import type { ImageView } from "@/api/upload";
import UploadQueueList from "./UploadQueueList.vue";
import UploadResultActions from "./UploadResultActions.vue";
import type { UploadQueueItem } from "./types";

const image: ImageView = {
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
  },
  local_thumb_url: "/t/2026/10/a.png",
  deleted_at: null,
  purge_at: null,
  created_at: "2026-10-04T00:00:00Z",
};

function makeItem(partial: Partial<UploadQueueItem>): UploadQueueItem {
  return {
    id: 1,
    file: new File(["x"], "a.png", { type: "image/png" }),
    name: "a.png",
    size: 2048,
    state: "queued",
    loaded: 0,
    total: 2048,
    error: null,
    image: null,
    ...partial,
  };
}

// 成功项内嵌 UploadResultActions，其 useMessage 需要 NMessageProvider 祖先
function mountList(items: UploadQueueItem[]) {
  const host = defineComponent({
    render: () => h(NMessageProvider, () => [h(UploadQueueList, { items })]),
  });
  return mount(host);
}

describe("UploadQueueList", () => {
  it("空队列显示占位", () => {
    const wrapper = mountList([]);
    expect(wrapper.text()).toContain("暂无待上传文件");
  });

  it("渲染四种状态标签与文件大小", () => {
    const wrapper = mountList([
      makeItem({ id: 1, state: "queued" }),
      makeItem({ id: 2, state: "uploading", loaded: 1024, total: 2048 }),
      makeItem({ id: 3, state: "success", image }),
      makeItem({ id: 4, state: "failed", error: { code: 30007, message: "不支持的图片格式", status: 415 } }),
    ]);

    expect(wrapper.text()).toContain("排队");
    expect(wrapper.text()).toContain("上传中");
    expect(wrapper.text()).toContain("成功");
    expect(wrapper.text()).toContain("失败（code 30007）：不支持的图片格式");
    expect(wrapper.text()).toContain("1.0 KB / 2.0 KB");
    expect(wrapper.text()).toContain("2.0 KB");
  });

  it("点击重试 emit retry 并携带队列项", async () => {
    const item = makeItem({ state: "failed", error: { code: 10001, message: "参数错误", status: 400 } });
    const wrapper = mountList([item]);

    await wrapper.find("button").trigger("click");

    const list = wrapper.findComponent(UploadQueueList);
    expect(list.emitted("retry")?.[0]).toEqual([item]);
  });

  it("成功项渲染复制操作，失败项没有", () => {
    const wrapper = mountList([
      makeItem({ id: 1, state: "success", image }),
      makeItem({ id: 2, state: "failed", error: { code: 30007, message: "格式不支持", status: 415 } }),
    ]);

    const actions = wrapper.findAllComponents(UploadResultActions);
    expect(actions.length).toBe(1);
    expect(wrapper.text()).toContain("复制");
    expect(wrapper.text()).toContain("重试");
  });

  it("上传中渲染进度条与字节进度", () => {
    const wrapper = mountList([makeItem({ state: "uploading", loaded: 512, total: 2048 })]);
    expect(wrapper.find(".n-progress").exists()).toBe(true);
    expect(wrapper.text()).toContain("512 B / 2.0 KB");
  });
});
