import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { NMessageProvider } from "naive-ui";
import { defineComponent, h, reactive } from "vue";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { getImageExif, type ImageExif, type ImageView } from "@/api/images";
import { makeExif, makeImage } from "./fixtures";
import ImageDetailPanel from "./ImageDetailPanel.vue";

vi.mock("@/api/images", () => ({ getImageExif: vi.fn() }));
const exifMock = vi.mocked(getImageExif);

enableAutoUnmount(afterEach);
beforeEach(() => {
  vi.resetAllMocks();
});

function mountPanel(image: ImageView | null) {
  const props = reactive<{ image: ImageView | null }>({ image });
  const wrapper = mount(
    defineComponent({
      setup: () => () => h(NMessageProvider, () => h(ImageDetailPanel, props)),
    }),
  );
  return { wrapper, props };
}

it("shows an empty state without an image and requests nothing", () => {
  const { wrapper } = mountPanel(null);
  expect(wrapper.find('[data-testid="detail-panel-empty"]').exists()).toBe(true);
  expect(exifMock).not.toHaveBeenCalled();
});

it("shows metadata and lazily loaded EXIF for the selected image", async () => {
  exifMock.mockResolvedValue(makeExif({ image_id: 7 }));
  const { wrapper } = mountPanel(makeImage({ id: 7, name: "sunset.jpg" }));
  await flushPromises();
  expect(exifMock).toHaveBeenCalledTimes(1);
  expect(exifMock).toHaveBeenCalledWith(7);
  expect(wrapper.find('[data-testid="detail-panel-title"]').text()).toBe("sunset.jpg");
  expect(wrapper.text()).toContain("Canon EOS R6");
  expect(wrapper.text()).toContain("仅本人可见");
});

it("loads EXIF once per image and ignores a stale response", async () => {
  const resolvers: ((value: ImageExif) => void)[] = [];
  exifMock.mockImplementation(() => new Promise((resolve) => resolvers.push(resolve)));
  const { wrapper, props } = mountPanel(makeImage({ id: 1 }));
  await flushPromises();
  props.image = makeImage({ id: 2 });
  await flushPromises();
  expect(exifMock).toHaveBeenCalledTimes(2);
  resolvers[1]!(makeExif({ image_id: 2, model: "B-camera" }));
  await flushPromises();
  resolvers[0]!(makeExif({ image_id: 1, model: "A-camera" }));
  await flushPromises();
  expect(wrapper.text()).toContain("B-camera");
  expect(wrapper.text()).not.toContain("A-camera");
});

it("does not reload EXIF when the same image object is replaced by an updated copy", async () => {
  exifMock.mockResolvedValue(makeExif({ image_id: 1 }));
  const { props } = mountPanel(makeImage({ id: 1, is_public: false }));
  await flushPromises();
  props.image = makeImage({ id: 1, is_public: true });
  await flushPromises();
  expect(exifMock).toHaveBeenCalledTimes(1);
});

it("clears EXIF when the image is cleared", async () => {
  exifMock.mockResolvedValue(makeExif({ image_id: 1 }));
  const { wrapper, props } = mountPanel(makeImage({ id: 1 }));
  await flushPromises();
  props.image = null;
  await flushPromises();
  expect(wrapper.text()).not.toContain("Canon EOS R6");
  expect(wrapper.find('[data-testid="detail-panel-empty"]').exists()).toBe(true);
});

it("emits close from the close button", async () => {
  exifMock.mockResolvedValue(makeExif());
  const { wrapper } = mountPanel(makeImage());
  await wrapper.find('[data-testid="detail-panel-close"]').trigger("click");
  expect(wrapper.findComponent(ImageDetailPanel).emitted("close")).toHaveLength(1);
});
