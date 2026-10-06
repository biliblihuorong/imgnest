import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, h, reactive } from "vue";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { fetchProtectedThumbnail } from "@/api/thumbnails";
import { useProtectedThumbnail } from "./useProtectedThumbnail";
vi.mock("@/api/thumbnails", () => ({ fetchProtectedThumbnail: vi.fn() }));
beforeEach(() => {
  vi.resetAllMocks();
  URL.createObjectURL = vi.fn().mockReturnValue("blob:new");
  URL.revokeObjectURL = vi.fn();
});
afterEach(() => vi.restoreAllMocks());
function mountThumbnail(state: { url: string }) {
  return mount(
    defineComponent({
      setup() {
        const { thumbnailUrl, failed } = useProtectedThumbnail(() => state.url);
        return () =>
          h("div", { "data-failed": failed.value }, [
            h("img", { src: thumbnailUrl.value }),
            thumbnailUrl.value,
          ]);
      },
    }),
  );
}
it("aborts outdated thumbnail requests and revokes each replaced URL", async () => {
  let resolveOld!: (blob: Blob) => void;
  vi.mocked(fetchProtectedThumbnail).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        resolveOld = resolve;
      }),
  );
  vi.mocked(fetchProtectedThumbnail).mockResolvedValueOnce(new Blob(["new"]));
  const state = reactive({ url: "/t/old.webp" });
  const wrapper = mountThumbnail(state);
  state.url = "/t/new.webp";
  await flushPromises();
  expect(vi.mocked(fetchProtectedThumbnail).mock.calls[0]?.[1]?.aborted).toBe(true);
  expect(wrapper.find("img").attributes("src")).toBe("blob:new");
  resolveOld(new Blob(["old"]));
  await flushPromises();
  expect(URL.createObjectURL).toHaveBeenCalledTimes(1);
  wrapper.unmount();
  expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:new");
});
it("removes a previous thumbnail immediately when the next image fails", async () => {
  vi.mocked(fetchProtectedThumbnail)
    .mockResolvedValueOnce(new Blob(["ok"]))
    .mockRejectedValueOnce(new Error("gone"));
  const state = reactive({ url: "/t/old.webp" });
  const wrapper = mountThumbnail(state);
  await flushPromises();
  expect(wrapper.text()).toBe("blob:new");
  state.url = "/t/new.webp";
  await flushPromises();
  expect(wrapper.text()).toBe("");
  expect(wrapper.attributes("data-failed")).toBe("true");
  wrapper.unmount();
});
it("revokes an existing preview immediately on session clear and removes its listener on unmount", async () => {
  vi.mocked(fetchProtectedThumbnail).mockResolvedValue(new Blob(["private"]));
  const remove = vi.spyOn(window, "removeEventListener");
  const wrapper = mountThumbnail(reactive({ url: "/t/private.webp" }));
  await flushPromises();
  window.dispatchEvent(new Event("imgnest:session-cleared"));
  await flushPromises();
  expect(wrapper.text()).toBe("");
  expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:new");
  wrapper.unmount();
  expect(remove).toHaveBeenCalledWith("imgnest:session-cleared", expect.any(Function));
  expect(URL.revokeObjectURL).toHaveBeenCalledTimes(1);
});
it("aborts pending private previews on session clear and ignores late completion", async () => {
  let resolve!: (blob: Blob) => void;
  vi.mocked(fetchProtectedThumbnail).mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  const wrapper = mountThumbnail(reactive({ url: "/t/private.webp" }));
  window.dispatchEvent(new Event("imgnest:session-cleared"));
  await flushPromises();
  expect(vi.mocked(fetchProtectedThumbnail).mock.calls[0]?.[1]?.aborted).toBe(true);
  resolve(new Blob(["private"]));
  await flushPromises();
  expect(URL.createObjectURL).not.toHaveBeenCalled();
  wrapper.unmount();
});
