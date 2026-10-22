import { i18n } from "@vben/locales";
import {
  enableAutoUnmount,
  flushPromises,
  mount,
  RouterLinkStub,
  type VueWrapper,
} from "@vue/test-utils";
import naive, { NMessageProvider, type MessageProviderInst } from "naive-ui";
import { createPinia, setActivePinia, type Pinia } from "pinia";
import { defineComponent, h, onBeforeUnmount, ref } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TOKEN_STORAGE_KEY } from "@/api/client";
import { listGallery } from "@/api/gallery";
import * as uploadApi from "@/api/upload";
import { makeImage } from "@/components/images/fixtures";
import { useSiteStore } from "@/stores/site";
import PublicHomeView from "./PublicHomeView.vue";

vi.mock("@/api/gallery", () => ({ listGallery: vi.fn() }));
vi.mock("@/api/auth", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/auth")>()),
  me: vi.fn(() => new Promise(() => {})),
}));
vi.mock("@/api/upload", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/upload")>()),
  uploadImages: vi.fn(),
  getGuestUploadLimits: vi.fn(),
}));

const listGalleryMock = vi.mocked(listGallery);
const uploadImagesMock = vi.mocked(uploadApi.uploadImages);
const guestLimitsMock = vi.mocked(uploadApi.getGuestUploadLimits);
enableAutoUnmount(afterEach);

let pinia: Pinia;

function mountView() {
  const host = defineComponent({
    setup() {
      const messages = ref<MessageProviderInst | null>(null);
      onBeforeUnmount(() => messages.value?.destroyAll());
      return () => h(NMessageProvider, { ref: messages }, () => [h(PublicHomeView)]);
    },
  });
  return mount(host, {
    global: { plugins: [naive, pinia], stubs: { RouterLink: RouterLinkStub } },
  });
}

function setSite(patch: { guestUpload?: boolean; galleryEnabled?: boolean }) {
  useSiteStore().$patch({ loaded: true, siteName: "Nest", ...patch });
}

async function chooseFile(wrapper: VueWrapper, file: File): Promise<void> {
  const input = wrapper.find("input[type='file']");
  Object.defineProperty(input.element, "files", { value: [file], configurable: true });
  await input.trigger("change");
  await flushPromises();
}

beforeEach(() => {
  i18n.global.locale.value = "zh-CN";
  vi.resetAllMocks();
  localStorage.clear();
  pinia = createPinia();
  setActivePinia(pinia);
  listGalleryMock.mockResolvedValue({ items: [], total: 0, page: 1, size: 12 });
  guestLimitsMock.mockResolvedValue({
    max_file_bytes: 5 * 1024 * 1024,
    max_files: 20,
    upload_per_min: 0,
  });
});

describe("PublicHomeView", () => {
  it("开放游客上传时，访客不登录也能上传并走游客接口", async () => {
    setSite({ guestUpload: true });
    const image = makeImage({ id: 9, name: "a.png" });
    uploadImagesMock.mockResolvedValue([{ ok: true, filename: "a.png", status: 201, image }]);
    const wrapper = mountView();
    await flushPromises();

    expect(wrapper.find("[data-test='guest-upload']").exists()).toBe(true);
    expect(guestLimitsMock).toHaveBeenCalledOnce();
    expect(wrapper.text()).toContain("5.0 MB");

    await chooseFile(wrapper, new File(["x"], "a.png", { type: "image/png" }));
    const start = wrapper.findAll("button").find((node) => node.text() === "开始上传")!;
    await start.trigger("click");
    await flushPromises();

    expect(uploadImagesMock).toHaveBeenCalledOnce();
    const options = uploadImagesMock.mock.calls[0]![0];
    expect(options.guest).toBe(true);
    expect(options.albumId).toBeNull();
    expect(options.isPublic).toBe(false);
  });

  it("未开放游客上传时提示登录，不请求游客上限", async () => {
    setSite({ guestUpload: false, galleryEnabled: true });
    const wrapper = mountView();
    await flushPromises();

    expect(wrapper.find("[data-test='guest-upload']").exists()).toBe(false);
    expect(wrapper.find("[data-test='sign-in-to-upload']").exists()).toBe(true);
    expect(guestLimitsMock).not.toHaveBeenCalled();
  });

  it("已登录用户看到进入控制台的入口而不是游客上传", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|token");
    setSite({ guestUpload: true });
    const wrapper = mountView();
    await flushPromises();

    expect(wrapper.find("[data-test='guest-upload']").exists()).toBe(false);
    expect(wrapper.text()).toContain("进入控制台");
  });

  it("跨标签登录、退出后入口随之切换，游客上传区仍可入队上传", async () => {
    setSite({ guestUpload: true });
    const image = makeImage({ id: 9, name: "a.png" });
    uploadImagesMock.mockResolvedValue([{ ok: true, filename: "a.png", status: 201, image }]);
    const wrapper = mountView();
    await flushPromises();
    const otherTab = async (token: string | null) => {
      if (token === null) localStorage.removeItem(TOKEN_STORAGE_KEY);
      else localStorage.setItem(TOKEN_STORAGE_KEY, token);
      window.dispatchEvent(
        new StorageEvent("storage", {
          key: TOKEN_STORAGE_KEY,
          newValue: token,
          storageArea: localStorage,
        }),
      );
      await flushPromises();
    };

    await otherTab("7|token");
    expect(wrapper.find("[data-test='guest-upload']").exists()).toBe(false);
    expect(wrapper.text()).toContain("进入控制台");

    await otherTab(null);
    expect(wrapper.find("[data-test='guest-upload']").exists()).toBe(true);
    expect(wrapper.text()).not.toContain("进入控制台");
    await chooseFile(wrapper, new File(["x"], "a.png", { type: "image/png" }));
    const start = wrapper.findAll("button").find((node) => node.text() === "开始上传")!;
    expect(start.attributes("disabled")).toBeUndefined();
    await start.trigger("click");
    await flushPromises();
    expect(uploadImagesMock).toHaveBeenCalledOnce();
    expect(uploadImagesMock.mock.calls[0]![0].guest).toBe(true);
  });

  it("开放画廊时预览最新 12 张公开图片", async () => {
    setSite({ galleryEnabled: true });
    const wrapper = mountView();
    await flushPromises();

    expect(listGalleryMock).toHaveBeenCalledWith({ page: 1, size: 12 });
    expect(wrapper.find("[data-test='gallery-preview']").exists()).toBe(true);
  });

  it("画廊未开放时不请求画廊", async () => {
    setSite({ guestUpload: true, galleryEnabled: false });
    const wrapper = mountView();
    await flushPromises();

    expect(listGalleryMock).not.toHaveBeenCalled();
    expect(wrapper.find("[data-test='gallery-preview']").exists()).toBe(false);
  });

  it("画廊加载失败时显示错误，重试成功后才显示空态", async () => {
    setSite({ galleryEnabled: true });
    listGalleryMock.mockRejectedValueOnce(new Error("offline"));
    const wrapper = mountView();
    await flushPromises();

    expect(wrapper.text()).toContain("画廊加载失败");
    expect(wrapper.text()).not.toContain("暂无公开图片");
    const retry = wrapper.findAll("button").find((node) => node.text() === "重试")!;
    await retry.trigger("click");
    await flushPromises();

    expect(wrapper.text()).not.toContain("画廊加载失败");
    expect(wrapper.text()).toContain("暂无公开图片");
  });

  it("关闭画廊后丢弃在途响应，重新开启时读取新内容", async () => {
    setSite({ galleryEnabled: true });
    let finish!: (page: Awaited<ReturnType<typeof listGallery>>) => void;
    listGalleryMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const wrapper = mountView();
    await flushPromises();
    useSiteStore().galleryEnabled = false;
    await flushPromises();
    finish({ items: [{ ...makeImage(), uploader: "old" }], total: 1, page: 1, size: 12 });
    await flushPromises();
    useSiteStore().galleryEnabled = true;
    await flushPromises();

    expect(wrapper.find(".gallery-card").exists()).toBe(false);
    expect(wrapper.text()).toContain("暂无公开图片");
    expect(listGalleryMock).toHaveBeenCalledTimes(2);
  });
});
