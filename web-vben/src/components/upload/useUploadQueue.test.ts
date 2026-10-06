import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { defineComponent } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TOKEN_STORAGE_KEY } from "@/api/client";
import { useUploadQueue } from "./useUploadQueue";

class DeferredXHR {
  static instances: DeferredXHR[] = [];
  status = 0;
  responseText = "";
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onabort: (() => void) | null = null;
  upload = { onprogress: null };
  aborted = false;
  constructor() {
    DeferredXHR.instances.push(this);
  }
  open() {}
  setRequestHeader() {}
  send() {}
  abort() {
    this.aborted = true;
    this.onabort?.();
  }
  respond() {
    this.status = 415;
    this.responseText = JSON.stringify({ code: 30007, message: "invalid image" });
    this.onload?.();
  }
}

let wrapper: VueWrapper;
function mountQueue() {
  let queue!: ReturnType<typeof useUploadQueue>;
  wrapper = mount(
    defineComponent({
      setup() {
        queue = useUploadQueue(() => ({ policyId: 1, albumId: 5, isPublic: false }));
        return () => null;
      },
    }),
  );
  return queue;
}

describe("upload queue session lifetime", () => {
  beforeEach(() => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "old-session");
    DeferredXHR.instances = [];
    vi.stubGlobal("XMLHttpRequest", DeferredXHR);
  });
  afterEach(() => {
    wrapper?.unmount();
    localStorage.clear();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("aborts the current request on unmount and never starts a second request", async () => {
    const queue = mountQueue();
    queue.onFiles([new File(["a"], "a.png"), new File(["b"], "b.png")]);
    queue.startUpload();
    const first = DeferredXHR.instances[0]!;
    wrapper.unmount();
    expect(first.aborted).toBe(true);
    first.respond();
    await flushPromises();
    expect(DeferredXHR.instances).toHaveLength(1);
    expect(queue.items.value.some((item) => item.state === "success")).toBe(false);
  });

  it("clears private queue state on session clear and cannot retry it under a new account", async () => {
    const queue = mountQueue();
    queue.onFiles([new File(["a"], "a.png"), new File(["b"], "b.png")]);
    const oldItem = queue.items.value[0]!;
    queue.startUpload();
    const first = DeferredXHR.instances[0]!;
    localStorage.removeItem(TOKEN_STORAGE_KEY);
    window.dispatchEvent(new Event("imgnest:session-cleared"));
    localStorage.setItem(TOKEN_STORAGE_KEY, "new-session");
    expect(first.aborted).toBe(true);
    first.respond();
    await flushPromises();
    expect(queue.items.value).toEqual([]);
    expect(queue.uploading.value).toBe(false);
    expect(queue.canUpload.value).toBe(false);
    queue.retryItem(oldItem);
    queue.startUpload();
    await flushPromises();
    expect(DeferredXHR.instances).toHaveLength(1);
  });

  it("removes the session listener when the queue unmounts", () => {
    const remove = vi.spyOn(window, "removeEventListener");
    mountQueue();
    wrapper.unmount();
    expect(remove).toHaveBeenCalledWith("imgnest:session-cleared", expect.any(Function));
  });
});
