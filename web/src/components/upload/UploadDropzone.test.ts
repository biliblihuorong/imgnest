import { mount, type VueWrapper } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import UploadDropzone from "./UploadDropzone.vue";

function makeFile(name: string): File {
  return new File(["x"], name, { type: "image/png" });
}

function mountDropzone(disabled = false) {
  return mount(UploadDropzone, { props: { disabled } });
}

/** test-utils 2.5 无 setInputFiles，手动挂 files 后触发 change。 */
async function chooseFiles(wrapper: VueWrapper, files: File[]): Promise<void> {
  const fileInput = wrapper.find("input[type='file']");
  Object.defineProperty(fileInput.element, "files", { value: files, configurable: true });
  await fileInput.trigger("change");
}

describe("UploadDropzone", () => {
  it("多选文件后 emit files 并清空 input", async () => {
    const wrapper = mountDropzone();
    const files = [makeFile("a.png"), makeFile("b.png")];

    await chooseFiles(wrapper, files);

    expect(wrapper.emitted("files")?.[0]).toEqual([files]);
    expect((wrapper.find("input[type='file']").element as HTMLInputElement).value).toBe("");
  });

  it("disabled 时不 emit files", async () => {
    const wrapper = mountDropzone(true);

    await chooseFiles(wrapper, [makeFile("a.png")]);
    await wrapper.find(".upload-dropzone").trigger("drop", { dataTransfer: { files: [makeFile("b.png")] } });

    expect(wrapper.emitted("files")).toBeUndefined();
  });

  it("drop 携带文件时 emit files", async () => {
    const wrapper = mountDropzone();
    const file = makeFile("drop.png");

    await wrapper.find(".upload-dropzone").trigger("drop", { dataTransfer: { files: [file] } });

    expect(wrapper.emitted("files")?.[0]).toEqual([[file]]);
  });

  it("drop 无文件时不 emit", async () => {
    const wrapper = mountDropzone();

    await wrapper.find(".upload-dropzone").trigger("drop", { dataTransfer: { files: [] } });

    expect(wrapper.emitted("files")).toBeUndefined();
  });

  it("页内粘贴文件时 emit files", async () => {
    const wrapper = mountDropzone();
    const file = makeFile("paste.png");
    const event = new Event("paste") as ClipboardEvent;
    Object.defineProperty(event, "clipboardData", { value: { files: [file] } });

    document.dispatchEvent(event);
    await wrapper.vm.$nextTick();

    expect(wrapper.emitted("files")?.[0]).toEqual([[file]]);
  });

  it("点击拖拽区触发隐藏 input 的文件选择", async () => {
    const clickSpy = vi.spyOn(HTMLInputElement.prototype, "click").mockImplementation(() => {});
    const wrapper = mountDropzone();

    await wrapper.find(".upload-dropzone").trigger("click");

    expect(clickSpy).toHaveBeenCalledTimes(1);
    clickSpy.mockRestore();
  });

  it("拖入时切换高亮样式", async () => {
    const wrapper = mountDropzone();

    await wrapper.find(".upload-dropzone").trigger("dragover");
    expect(wrapper.find(".upload-dropzone").classes()).toContain("is-dragging");

    await wrapper.find(".upload-dropzone").trigger("dragleave");
    expect(wrapper.find(".upload-dropzone").classes()).not.toContain("is-dragging");
  });
});
