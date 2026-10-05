import { i18n } from "@vben/locales";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import naive, { NMessageProvider, NSelect } from "naive-ui";
import { defineComponent, h } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/api/client";
import { listAlbums } from "@/api/albums";
import { makeAlbum } from "@/components/albums/fixtures";
import * as policiesApi from "@/api/policies";
import * as uploadApi from "@/api/upload";
import type { ImageView, UploadItemResult } from "@/api/upload";
import UploadResultActions from "@/components/upload/UploadResultActions.vue";
import UploadView from "./UploadView.vue";

vi.mock("@/api/policies", () => ({
  listPolicies: vi.fn(),
}));

vi.mock("@/api/albums", () => ({
  listAlbums: vi.fn(),
}));

vi.mock("@/api/upload", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/upload")>()),
  uploadImages: vi.fn(),
}));

const policiesApiMock = vi.mocked(policiesApi);
const listAlbumsMock = vi.mocked(listAlbums);
const uploadImagesMock = vi.mocked(uploadApi.uploadImages);

function makeFile(name: string, size: number): File {
  const file = new File(["x"], name, { type: "image/png" });
  Object.defineProperty(file, "size", { value: size });
  return file;
}

function makeImage(name: string): ImageView {
  return {
    id: 7,
    key: `2026/10/${name}`,
    user_id: 1,
    album_id: 0,
    policy_id: 1,
    storage_id: 1,
    name,
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
      url: `https://img.example/2026/10/${name}`,
      original: `https://img.example/2026/10/${name}`,
      webp: `https://img.example/2026/10/${name.replace(/\.png$/, ".webp")}`,
      thumbnail_url: `https://img.example/2026/10/t_${name}`,
    },
    local_thumb_url: `/t/2026/10/${name}`,
    deleted_at: null,
    purge_at: null,
    created_at: "2026-10-04T00:00:00Z",
  };
}

// useMessage 需要 NMessageProvider 祖先；naive-ui 组件按提示整体挂载
function mountView() {
  const host = defineComponent({
    render: () => h(NMessageProvider, () => [h(UploadView)]),
  });
  return mount(host, { global: { plugins: [naive] } });
}

async function chooseFiles(wrapper: VueWrapper, files: File[]): Promise<void> {
  const fileInput = wrapper.find("input[type='file']");
  Object.defineProperty(fileInput.element, "files", { value: files, configurable: true });
  await fileInput.trigger("change");
  await flushPromises();
}

function findButton(wrapper: VueWrapper, text: string) {
  const button = wrapper.findAll("button").find((node) => node.text() === text);
  if (!button) {
    throw new Error(`button ${text} not found`);
  }
  return button;
}

async function startUpload(wrapper: VueWrapper): Promise<void> {
  await findButton(wrapper, "开始上传").trigger("click");
  await flushPromises();
}

/** 按文本选中队列头部的全局复制格式/版本分段按钮（NRadioButton，native input）。 */
async function selectSegmented(wrapper: VueWrapper, label: string): Promise<void> {
  const radio = wrapper.findAll(".n-radio-button").find((node) => node.text().trim() === label);
  if (!radio) {
    throw new Error(`segmented option ${label} not found`);
  }
  await radio.find("input").setValue(true);
}

describe("UploadView", () => {
  it("页面提供唯一主标题，并让上传队列计数随所选文件更新", async () => {
    policiesApiMock.listPolicies.mockResolvedValue([{ id: 3, name: "默认规则" }]);
    const wrapper = mountView();
    await flushPromises();

    expect(wrapper.findAll("h1")).toHaveLength(1);
    expect(wrapper.find("h1").text()).toBe("上传图片");
    expect(wrapper.find("[aria-label='上传队列数量']").text()).toBe("0 个文件");

    await chooseFiles(wrapper, [makeFile("a.png", 100), makeFile("b.png", 100)]);

    expect(wrapper.find("[aria-label='上传队列数量']").text()).toBe("2 个文件");
    expect(findButton(wrapper, "开始上传").attributes("disabled")).toBeUndefined();
  });
  beforeEach(() => {
    // NSelect/NSwitch 依赖 ResizeObserver，jsdom 没有
    vi.stubGlobal(
      "ResizeObserver",
      class {
        observe(): void {}
        unobserve(): void {}
        disconnect(): void {}
      },
    );
    listAlbumsMock.mockResolvedValue({
      items: [makeAlbum({ id: 5, name: "旅行" })],
      total: 1,
      page: 1,
      size: 100,
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.resetAllMocks();
  });

  it("预检：文件数超过 20 拒绝且不发请求", async () => {
    const wrapper = mountView();
    await flushPromises();
    const files = Array.from({ length: 21 }, (_, i) => makeFile(`f${i}.png`, 100));

    await chooseFiles(wrapper, files);

    expect(wrapper.text()).toContain("单次最多上传 20 个文件，当前选择了 21 个");
    expect(uploadImagesMock).not.toHaveBeenCalled();
  });

  it("预检：单文件超过 20MiB 拒绝且不发请求", async () => {
    const wrapper = mountView();
    await flushPromises();

    await chooseFiles(wrapper, [makeFile("small.png", 100), makeFile("big.png", 21 * 1024 * 1024)]);

    expect(wrapper.text()).toContain("单文件上限");
    expect(wrapper.text()).toContain("big.png");
    expect(wrapper.text()).not.toContain("small.png");
    expect(uploadImagesMock).not.toHaveBeenCalled();
  });

  it("预检：合计超过 64MiB 拒绝且不发请求", async () => {
    const wrapper = mountView();
    await flushPromises();

    const half = 32 * 1024 * 1024 + 1;
    await chooseFiles(wrapper, [makeFile("a.png", half), makeFile("b.png", half)]);

    expect(wrapper.text()).toContain("单次合计上限");
    expect(uploadImagesMock).not.toHaveBeenCalled();
  });

  it("单文件 201 成功后展示复制操作，URL 与 Markdown 拼接正确", async () => {
    policiesApiMock.listPolicies.mockResolvedValue([{ id: 3, name: "默认规则" }]);
    const image = makeImage("a.png");
    uploadImagesMock.mockResolvedValue([{ ok: true, filename: "a.png", status: 201, image }]);
    const wrapper = mountView();
    await flushPromises();
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal("navigator", { userAgent: "vitest", clipboard: { writeText } });

    await chooseFiles(wrapper, [makeFile("a.png", 1024)]);
    await startUpload(wrapper);

    expect(uploadImagesMock).toHaveBeenCalledTimes(1);
    const options = uploadImagesMock.mock.calls[0]?.[0];
    expect(options?.policyId).toBe(3);
    expect(options?.isPublic).toBe(false);
    expect(options?.files.length).toBe(1);

    const actions = wrapper.findComponent(UploadResultActions);
    expect(actions.exists()).toBe(true);

    // 快捷复制跟随全局默认：WebP + URL
    await findButton(actions, "复制").trigger("click");
    await flushPromises();
    expect(writeText).toHaveBeenCalledWith(image.links.webp);

    // 头部全局切换到 Markdown，行内复制立即跟随
    await selectSegmented(wrapper, "Markdown");
    await findButton(actions, "复制").trigger("click");
    await flushPromises();
    expect(writeText).toHaveBeenLastCalledWith(`![a.png](${image.links.webp})`);

    // 头部全局切回原图（格式仍是 Markdown），行内复制跟随
    await selectSegmented(wrapper, "原图");
    await findButton(actions, "复制").trigger("click");
    await flushPromises();
    expect(writeText).toHaveBeenLastCalledWith(`![a.png](${image.links.original})`);
  });

  it("207 部分失败逐项呈现：成功项可复制、失败项有 code/message 与重试", async () => {
    policiesApiMock.listPolicies.mockResolvedValue([{ id: 1, name: "规则一" }]);
    const fileA = makeFile("a.png", 100);
    const fileB = makeFile("b.png", 200);
    uploadImagesMock.mockResolvedValueOnce([
      { ok: true, filename: "a.png", status: 201, image: makeImage("a.png") },
      { ok: false, filename: "b.png", status: 415, code: 30007, message: "不支持的图片格式" },
    ]);
    const wrapper = mountView();
    await flushPromises();

    await chooseFiles(wrapper, [fileA, fileB]);
    await startUpload(wrapper);

    expect(wrapper.text()).toContain("失败（code 30007）：不支持该图片格式");
    expect(wrapper.findComponent(UploadResultActions).exists()).toBe(true);
    expect(findButton(wrapper, "重试").exists()).toBe(true);

    // 重试单个失败文件
    uploadImagesMock.mockResolvedValueOnce([
      { ok: true, filename: "b.png", status: 201, image: makeImage("b.png") },
    ]);
    await findButton(wrapper, "重试").trigger("click");
    await flushPromises();

    expect(uploadImagesMock).toHaveBeenCalledTimes(2);
    expect(uploadImagesMock.mock.calls[1]?.[0]?.files).toEqual([fileB]);
    expect(wrapper.text()).not.toContain("不支持的图片格式");
    expect(wrapper.findAllComponents(UploadResultActions).length).toBe(2);
  });

  it("policies 为空时显示引导文案并禁用上传", async () => {
    policiesApiMock.listPolicies.mockResolvedValue([]);
    const wrapper = mountView();
    await flushPromises();

    expect(wrapper.text()).toContain("当前用户组没有可用的上传规则，请联系管理员");

    await chooseFiles(wrapper, [makeFile("a.png", 100)]);
    expect(findButton(wrapper, "开始上传").attributes("disabled")).toBeDefined();
    expect(uploadImagesMock).not.toHaveBeenCalled();
  });

  it("恰好一条规则时自动选中", async () => {
    policiesApiMock.listPolicies.mockResolvedValue([{ id: 5, name: "唯一规则" }]);
    const wrapper = mountView();
    await flushPromises();

    expect(wrapper.text()).toContain("唯一规则");
  });

  it("多条规则时不自动选中，等待用户选择", async () => {
    policiesApiMock.listPolicies.mockResolvedValue([
      { id: 1, name: "规则一" },
      { id: 2, name: "规则二" },
    ]);
    const wrapper = mountView();
    await flushPromises();

    await chooseFiles(wrapper, [makeFile("a.png", 100)]);
    expect(findButton(wrapper, "开始上传").attributes("disabled")).toBeDefined();
  });

  it("policies 加载失败时显示错误信息并禁用上传", async () => {
    policiesApiMock.listPolicies.mockRejectedValue(new ApiError(20001, "登录已过期", 401));
    const wrapper = mountView();
    await flushPromises();

    expect(wrapper.text()).toContain("登录已过期");
    await chooseFiles(wrapper, [makeFile("a.png", 100)]);
    expect(findButton(wrapper, "开始上传").attributes("disabled")).toBeDefined();
    expect(uploadImagesMock).not.toHaveBeenCalled();
  });

  it("公开开关随用户切换传入上传选项", async () => {
    policiesApiMock.listPolicies.mockResolvedValue([{ id: 1, name: "规则一" }]);
    uploadImagesMock.mockResolvedValue([] as UploadItemResult[]);
    const wrapper = mountView();
    await flushPromises();

    const sw = wrapper.find(".n-switch");
    await sw.trigger("click");
    await chooseFiles(wrapper, [makeFile("a.png", 100)]);
    await startUpload(wrapper);

    expect(uploadImagesMock.mock.calls[0]?.[0]?.isPublic).toBe(true);
  });

  it("选择相册后上传选项带 album_id，清除后回到 null", async () => {
    policiesApiMock.listPolicies.mockResolvedValue([{ id: 1, name: "规则一" }]);
    uploadImagesMock.mockResolvedValue([] as UploadItemResult[]);
    const wrapper = mountView();
    await flushPromises();

    // 相册选择器是第二个 NSelect（第一个是上传规则）
    const selects = wrapper.findAllComponents(NSelect);
    expect(selects.length).toBe(2);
    await selects[1]!.vm.$emit("update:value", 5);

    await chooseFiles(wrapper, [makeFile("a.png", 100)]);
    await startUpload(wrapper);
    expect(uploadImagesMock.mock.calls[0]?.[0]?.albumId).toBe(5);

    await selects[1]!.vm.$emit("update:value", null);
    await chooseFiles(wrapper, [makeFile("b.png", 100)]);
    await startUpload(wrapper);
    expect(uploadImagesMock.mock.calls[1]?.[0]?.albumId).toBeNull();
  });
});

describe("UploadView migration regressions", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    policiesApiMock.listPolicies.mockResolvedValue([{ id: 1, name: "Default" }]);
  });
  it("translates labels and preflight errors after a live locale switch", async () => {
    const wrapper = mountView();
    await flushPromises();
    await chooseFiles(
      wrapper,
      Array.from({ length: 21 }, (_, i) => makeFile(`${i}.png`, 100)),
    );
    i18n.global.locale.value = "en-US";
    await flushPromises();
    expect(wrapper.find("h1").text()).toBe("Upload images");
    expect(wrapper.text()).toContain("At most 20 files per upload; 21 selected");
    expect(wrapper.text()).not.toContain("单次最多上传");
    wrapper.unmount();
  });
  it("checks the combined pending queue before accepting another selection", async () => {
    const wrapper = mountView();
    await flushPromises();
    await chooseFiles(
      wrapper,
      Array.from({ length: 15 }, (_, i) => makeFile(`${i}.png`, 100)),
    );
    await chooseFiles(
      wrapper,
      Array.from({ length: 6 }, (_, i) => makeFile(`new${i}.png`, 100)),
    );
    expect(wrapper.findAll(".upload-queue__item")).toHaveLength(15);
    expect(wrapper.text()).toContain("当前选择了 21 个");
    wrapper.unmount();
  });
  it("turns unexpected upload rejection into retryable queue errors", async () => {
    uploadImagesMock.mockRejectedValue(new Error("transport lost"));
    const wrapper = mountView();
    await flushPromises();
    await chooseFiles(wrapper, [makeFile("a.png", 100)]);
    await startUpload(wrapper);
    expect(findButton(wrapper, "重试").exists()).toBe(true);
    expect(findButton(wrapper, "开始上传").attributes("disabled")).toBeDefined();
    wrapper.unmount();
  });
});

it("does not upload twice when Start is clicked repeatedly", async () => {
  policiesApiMock.listPolicies.mockResolvedValue([{ id: 1, name: "Default" }]);
  uploadImagesMock.mockReturnValue(new Promise(() => {}));
  const wrapper = mountView();
  await flushPromises();
  await chooseFiles(wrapper, [makeFile("a.png", 100)]);
  const button = findButton(wrapper, "开始上传");
  await button.trigger("click");
  await button.trigger("click");
  await flushPromises();
  expect(uploadImagesMock).toHaveBeenCalledTimes(1);
  wrapper.unmount();
});
it("can retry loading upload rules after a transient failure without losing the queue", async () => {
  policiesApiMock.listPolicies
    .mockRejectedValueOnce(new Error("offline"))
    .mockResolvedValueOnce([{ id: 8, name: "Recovered" }]);
  const wrapper = mountView();
  await flushPromises();
  await chooseFiles(wrapper, [makeFile("a.png", 100)]);
  const retry = wrapper
    .find(".upload-view__policy-field")
    .findAll("button")
    .find((button) => button.text() === "重试");
  expect(retry).toBeDefined();
  await retry!.trigger("click");
  await flushPromises();
  expect(wrapper.text()).toContain("a.png");
  expect(wrapper.text()).toContain("Recovered");
  expect(findButton(wrapper, "开始上传").attributes("disabled")).toBeUndefined();
  wrapper.unmount();
});
