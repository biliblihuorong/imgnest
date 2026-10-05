import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NMessageProvider } from "naive-ui";
import { defineComponent, h, reactive } from "vue";
import { getImageExif } from "@/api/images";
import type { ImageView } from "@/api/images";
import { makeExif, makeImage } from "./fixtures";
import ImageDetailDrawer from "./ImageDetailDrawer.vue";

vi.mock("@/api/images", () => ({
  getImageExif: vi.fn(),
}));

const exifMock = vi.mocked(getImageExif);

enableAutoUnmount(afterEach);

function mountDrawer(image: ImageView) {
  const props = reactive({ show: false, image });
  const harness = defineComponent({
    setup: () => () => h(NMessageProvider, () => h(ImageDetailDrawer, props)),
  });
  const wrapper = mount(harness);
  return { wrapper, props };
}

beforeEach(() => {
  vi.resetAllMocks();
});

describe("ImageDetailDrawer", () => {
  it("详情抽屉宽度受视口约束，窄屏仍可关闭", async () => {
    exifMock.mockResolvedValue(makeExif());
    const { wrapper, props } = mountDrawer(makeImage());
    props.show = true;
    await flushPromises();

    const drawer = document.body.querySelector<HTMLElement>(".n-drawer");
    expect(drawer?.style.width).toBe("min(520px, 100vw)");
    const close = document.body.querySelector<HTMLElement>(".n-drawer-header__close");
    expect(close).not.toBeNull();
    close?.click();
    await flushPromises();
    expect(wrapper.findComponent(ImageDetailDrawer).emitted("update:show")).toEqual([[false]]);
  });

  it("关闭状态不请求 EXIF（懒加载）", () => {
    mountDrawer(makeImage({ id: 7 }));

    expect(exifMock).not.toHaveBeenCalled();
  });

  it("打开时才请求 EXIF，展示相机参数与 GPS 仅本人可见标注", async () => {
    exifMock.mockResolvedValue(makeExif({ image_id: 7 }));
    const { props } = mountDrawer(makeImage({ id: 7 }));

    props.show = true;
    await flushPromises();

    expect(exifMock).toHaveBeenCalledTimes(1);
    expect(exifMock).toHaveBeenCalledWith(7);
    const text = document.body.textContent ?? "";
    expect(text).toContain("EXIF 信息");
    expect(text).toContain("Canon EOS R6");
    expect(text).toContain("1/250");
    expect(text).toContain("GPS 定位");
    expect(text).toContain("仅本人可见");
    expect(text).toContain("纬度 31.2304");
  });

  it("无 GPS 时展示无 GPS 信息", async () => {
    exifMock.mockResolvedValue(makeExif({ gps_lat: null, gps_lng: null, gps_alt: null }));
    const { props } = mountDrawer(makeImage());

    props.show = true;
    await flushPromises();

    expect(document.body.textContent).toContain("无 GPS 信息");
  });

  it("raw 完整元数据折叠展示原文", async () => {
    exifMock.mockResolvedValue(makeExif({ raw: { Make: "Canon", Model: "EOS R6" } }));
    const { props } = mountDrawer(makeImage());

    props.show = true;
    await flushPromises();

    const header = document.body.querySelector(".n-collapse-item__header-main");
    expect(header).not.toBeNull();
    (header as HTMLElement).click();
    await flushPromises();

    const rawBlock = document.body.querySelector(".drawer-raw");
    expect(rawBlock?.textContent).toContain('"Make": "Canon"');
  });

  it("EXIF 请求失败时给出失败提示", async () => {
    exifMock.mockRejectedValue(new Error("网络错误"));
    const { props } = mountDrawer(makeImage());

    props.show = true;
    await flushPromises();

    expect(document.body.textContent).toContain("EXIF 加载失败");
  });

  it("展示基础元数据（大小/计费大小/src_md5）", async () => {
    exifMock.mockResolvedValue(makeExif());
    const { props } = mountDrawer(makeImage({ size: 2048, charged_bytes: 3072, src_md5: "abc" }));

    props.show = true;
    await flushPromises();

    const text = document.body.textContent ?? "";
    expect(text).toContain("2.0 KB");
    expect(text).toContain("3.0 KB");
    expect(text).toContain("abc");
  });
});

it("does not replace current private EXIF with a late previous-image response", async () => {
  let resolveOld!: (value: Awaited<ReturnType<typeof getImageExif>>) => void;
  exifMock.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        resolveOld = resolve;
      }),
  );
  exifMock.mockResolvedValueOnce(makeExif({ image_id: 2, make: "NEW-CAMERA" }));
  const { props } = mountDrawer(makeImage({ id: 1 }));
  props.show = true;
  await flushPromises();
  props.image = makeImage({ id: 2 });
  await flushPromises();
  resolveOld(makeExif({ image_id: 1, make: "OLD-PRIVATE-CAMERA" }));
  await flushPromises();
  expect(document.body.textContent).toContain("NEW-CAMERA");
  expect(document.body.textContent).not.toContain("OLD-PRIVATE-CAMERA");
});

it("ignores a pending EXIF result after close and only displays the reopened image", async () => {
  let resolveOld!: (value: Awaited<ReturnType<typeof getImageExif>>) => void;
  exifMock.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        resolveOld = resolve;
      }),
  );
  exifMock.mockResolvedValueOnce(makeExif({ make: "CURRENT" }));
  const { props } = mountDrawer(makeImage());
  props.show = true;
  await flushPromises();
  props.show = false;
  await flushPromises();
  props.show = true;
  await flushPromises();
  resolveOld(makeExif({ make: "CLOSED-PRIVATE" }));
  await flushPromises();
  expect(document.body.textContent).toContain("CURRENT");
  expect(document.body.textContent).not.toContain("CLOSED-PRIVATE");
});
