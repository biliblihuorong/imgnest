import { flushPromises, mount } from "@vue/test-utils";
import { h } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useImageQueryState } from "./useImageQueryState";
import { searchImages } from "@/api/images";
import { makeImage } from "./fixtures";
vi.mock("@/api/images", () => ({ searchImages: vi.fn() }));
const api = vi.mocked(searchImages);
const cleanups: (() => void)[] = [];
afterEach(() => {
  cleanups.splice(0).forEach((x) => x());
  vi.resetAllMocks();
  localStorage.clear();
});
const response = (q = "", page = 1, name = "fresh.png") => ({
  items: [makeImage({ name })],
  total: 60,
  page,
  size: 20,
  search: {
    appliedVersion: 1 as const,
    canonicalQ: q,
    tz: "UTC",
    authorizedAlbums: [],
    appliedRange: { afterUtc: null, beforeUtc: null },
  },
});
async function setup(url = "/images?qv=1&q=&tz=UTC&page=1&size=20") {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/images", component: { render: () => h("div") } }],
  });
  await router.push(url);
  await router.isReady();
  let state!: ReturnType<typeof useImageQueryState>;
  const wrapper = mount(
    {
      setup() {
        state = useImageQueryState({});
        return () => h("div");
      },
    },
    { global: { plugins: [router] } },
  );
  cleanups.push(() => wrapper.unmount());
  await flushPromises();
  return { state, router };
}
describe("applied image query state", () => {
  it("keeps input local and refresh reruns applied rather than draft", async () => {
    api.mockResolvedValue(response());
    const { state } = await setup();
    state.draftRaw.value = "camera:canon";
    await flushPromises();
    expect(api).toHaveBeenCalledTimes(1);
    await state.load();
    expect(api.mock.calls.at(-1)?.[0].q).toBe("");
    expect(state.draftRaw.value).toBe("camera:canon");
  });
  it("preserves successful results and draft on parse and protocol errors", async () => {
    api.mockResolvedValue(response());
    const { state } = await setup();
    state.draftRaw.value = "format:";
    await state.submit();
    expect(api).toHaveBeenCalledTimes(1);
    expect(state.images.value[0]?.name).toBe("fresh.png");
    state.draftRaw.value = "format:jpg";
    api.mockResolvedValue({ ...response(), search: undefined } as never);
    await state.submit();
    expect(state.diagnostics.value[0]?.code).toBe("SEARCH_PROTOCOL_MISMATCH");
    expect(state.images.value[0]?.name).toBe("fresh.png");
    expect(state.draftRaw.value).toBe("format:jpg");
  });
  it("canonicalizes one pushed query, resets page and restores back navigation", async () => {
    api.mockImplementation(async (p) => response(p.q.includes("旅行") ? "album:#42" : p.q, p.page));
    const { state, router } = await setup();
    state.draftRaw.value = "album:旅行";
    await state.submit();
    await flushPromises();
    expect(router.currentRoute.value.query.q).toBe("album:#42");
    await state.handlePageChange(3);
    expect(state.page.value).toBe(3);
    router.back();
    await flushPromises();
    expect(state.page.value).toBe(1);
    expect(state.draftRaw.value).toBe("album:#42");
  });
  it("old responses and old finally cannot replace newer query state", async () => {
    api.mockResolvedValue(response());
    const { state } = await setup();
    let finishA!: (x: ReturnType<typeof response>) => void;
    let finishB!: (x: ReturnType<typeof response>) => void;
    api
      .mockImplementationOnce(
        () =>
          new Promise((r) => {
            finishA = r;
          }),
      )
      .mockImplementationOnce(
        () =>
          new Promise((r) => {
            finishB = r;
          }),
      );
    state.draftRaw.value = "A";
    const a = state.submit();
    state.draftRaw.value = "B";
    const b = state.submit();
    finishA(response('"a"', 1, "stale.png"));
    await a;
    expect(state.loading.value).toBe(true);
    finishB(response('"b"', 1, "new.png"));
    await b;
    expect(state.images.value[0]?.name).toBe("new.png");
  });
  it("invalid URL cannot silently query the full library", async () => {
    await setup("/images?qv=2&q=bad&tz=UTC&page=1&size=20");
    expect(api).not.toHaveBeenCalled();
  });
  it("logout removes private images and ignores late results", async () => {
    let finish!: (x: ReturnType<typeof response>) => void;
    api.mockImplementationOnce(
      () =>
        new Promise((r) => {
          finish = r;
        }),
    );
    const { state } = await setup();
    window.dispatchEvent(new Event("imgnest:session-cleared"));
    finish(response());
    await flushPromises();
    expect(state.images.value).toEqual([]);
    expect(state.loading.value).toBe(false);
  });
});

import { validTimezone } from "./useImageQueryState";
it.each(["UTC", "CET", "EST5EDT", "GMT", "Asia/Shanghai", "America/New_York", "Etc/GMT+8"])(
  "accepts valid IANA timezone %s",
  (value) => {
    expect(validTimezone(value)).toBe(true);
  },
);
it.each(["", "Local", "GMT+8", "UTC+08:00", "+08:00", "Bad/Zone"])(
  "rejects invalid or implicit timezone %s",
  (value) => {
    expect(validTimezone(value)).toBe(false);
  },
);

it("keeps invalid URL diagnostics visible and refresh cannot silently fetch unfiltered images", async () => {
  const { state } = await setup("/images?qv=2&q=summer&tz=UTC&page=1&size=20");
  expect(state.diagnostics.value[0]?.code).toBe("UNSUPPORTED_QUERY_VERSION");
  await state.load();
  expect(api).not.toHaveBeenCalled();
});
it("preserves unapplied draft across pagination and retry of a different request", async () => {
  api.mockImplementation(async (p) => response(p.q, p.page));
  const { state } = await setup();
  state.draftRaw.value = "untouched draft";
  await state.handlePageChange(2);
  expect(state.draftRaw.value).toBe("untouched draft");
  api.mockRejectedValueOnce(new Error("offline"));
  await state.handlePageChange(3);
  state.draftRaw.value = "newer draft";
  await state.retry();
  expect(state.draftRaw.value).toBe("newer draft");
  expect(state.page.value).toBe(3);
});
it("preserves submitted state and server error span until the draft changes", async () => {
  api.mockResolvedValue(response());
  const { state } = await setup();
  state.draftRaw.value = "album:旅行";
  const { ApiError } = await import("@/api/client");
  api.mockRejectedValueOnce(
    new ApiError(10001, "ambiguous", 422, {
      diagnostics: [
        {
          code: "ALBUM_AMBIGUOUS",
          messageKey: "search.error.albumAmbiguous",
          span: { start: 6, end: 8 },
          args: {},
        },
      ],
    }),
  );
  await state.submit();
  expect(state.submitted.value).toBe(true);
  expect(state.diagnostics.value[0]?.span).toEqual({ start: 6, end: 8 });
  state.draftRaw.value = "album:#42";
  await flushPromises();
  expect(state.diagnostics.value).toEqual([]);
});

it("sends the original source so resolver diagnostics select the right draft token", async () => {
  api.mockResolvedValue(response());
  const { state } = await setup();
  state.draftRaw.value = "camera:canon album:旅行";
  const { ApiError } = await import("@/api/client");
  api.mockRejectedValueOnce(
    new ApiError(10001, "album unavailable", 422, {
      diagnostics: [
        {
          code: "ALBUM_NOT_AVAILABLE",
          messageKey: "search.error.albumNotAvailable",
          span: { start: 19, end: 21 },
          args: {},
        },
      ],
    }),
  );
  await state.submit();
  expect(api.mock.calls.at(-1)?.[0].q).toBe("camera:canon album:旅行");
  expect(
    state.draftRaw.value.slice(
      state.diagnostics.value[0]!.span.start,
      state.diagnostics.value[0]!.span.end,
    ),
  ).toBe("旅行");
});
it("does not attach a submitted source error span to a newer draft", async () => {
  api.mockResolvedValue(response());
  const { state } = await setup();
  let fail!: (e: unknown) => void;
  api.mockImplementationOnce(
    () =>
      new Promise((_, r) => {
        fail = r;
      }),
  );
  state.draftRaw.value = "album:旅行";
  const pending = state.submit();
  state.draftRaw.value = "new draft";
  const { ApiError } = await import("@/api/client");
  fail(
    new ApiError(10001, "unavailable", 422, {
      diagnostics: [
        {
          code: "ALBUM_NOT_AVAILABLE",
          messageKey: "search.error.albumNotAvailable",
          span: { start: 6, end: 8 },
          args: {},
        },
      ],
    }),
  );
  await pending;
  expect(state.draftRaw.value).toBe("new draft");
  expect(state.diagnostics.value).toEqual([]);
});

it("bounds a stuck latest request and exposes a retry without losing successful results", async () => {
  api.mockResolvedValue(response());
  const { state } = await setup();
  vi.useFakeTimers();
  api.mockImplementationOnce(() => new Promise(() => {}));
  state.draftRaw.value = "summer";
  const pending = state.submit();
  await vi.advanceTimersByTimeAsync(30000);
  await pending;
  expect(state.loading.value).toBe(false);
  expect(state.loadError.value).not.toBeNull();
  expect(state.images.value[0]?.name).toBe("fresh.png");
  vi.useRealTimers();
  await state.retry();
  expect(state.loadError.value).toBeNull();
});
