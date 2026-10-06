import { flushPromises, mount } from "@vue/test-utils";
import { h, ref } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";
import { completionContext, useSearchSuggestions } from "./useSearchSuggestions";
import { suggestAlbums } from "@/api/albums";
vi.mock("@/api/albums", () => ({ suggestAlbums: vi.fn() }));
afterEach(() => {
  vi.useRealTimers();
  vi.resetAllMocks();
  localStorage.clear();
});
it("completion replaces only the active quoted/comma list item", () => {
  expect(completionContext('album:"A,B",wo,#42', 14)).toEqual({
    field: "album",
    prefix: "wo",
    span: { start: 12, end: 14 },
  });
  expect(completionContext("format:jpg,pn,webp", 13)).toEqual({
    field: "format",
    prefix: "pn",
    span: { start: 11, end: 13 },
  });
});
describe("session-local album suggestions", () => {
  it("discards old keyword and scope responses even when abort is ignored", async () => {
    vi.useFakeTimers();
    let finishA!: (x: { items: { id: string; name: string }[]; hasMore: boolean }) => void;
    const api = vi.mocked(suggestAlbums);
    api
      .mockImplementationOnce(
        () =>
          new Promise((r) => {
            finishA = r;
          }),
      )
      .mockResolvedValue({ items: [{ id: "57", name: "B" }], hasMore: false });
    const raw = ref("album:a"),
      scope = ref<number | string | undefined>(42),
      caret = ref(7);
    let result!: ReturnType<typeof useSearchSuggestions>;
    const wrapper = mount({
      setup() {
        result = useSearchSuggestions({
          raw,
          scopeAlbumId: scope,
          caret,
          focused: ref(true),
          composing: ref(false),
        });
        return () => h("div");
      },
    });
    await vi.advanceTimersByTimeAsync(250);
    raw.value = "album:b";
    scope.value = 57;
    await flushPromises();
    await vi.advanceTimersByTimeAsync(250);
    await flushPromises();
    finishA({ items: [{ id: "42", name: "private old" }], hasMore: true });
    await flushPromises();
    expect(result.items.value.map((x) => x.label)).toEqual(["B (#57)"]);
    expect(result.hasMore.value).toBe(false);
    window.dispatchEvent(new Event("imgnest:session-cleared"));
    expect(result.items.value).toEqual([]);
    wrapper.unmount();
  });
  it("uses paginated server results rather than a local first-100 album cache", async () => {
    vi.useFakeTimers();
    const api = vi.mocked(suggestAlbums);
    api
      .mockResolvedValueOnce({ items: [{ id: "1000", name: "First" }], hasMore: true })
      .mockResolvedValueOnce({ items: [{ id: "2000", name: "Next" }], hasMore: false });
    let result!: ReturnType<typeof useSearchSuggestions>;
    const wrapper = mount({
      setup() {
        result = useSearchSuggestions({
          raw: ref("album:"),
          scopeAlbumId: ref(undefined),
          caret: ref(6),
          focused: ref(true),
          composing: ref(false),
        });
        return () => h("div");
      },
    });
    await vi.advanceTimersByTimeAsync(250);
    await result.more();
    expect(api.mock.calls[1]?.[1]).toBe(2);
    expect(result.items.value.map((x) => x.insert)).toEqual(["#1000", "#2000"]);
    wrapper.unmount();
  });
});
