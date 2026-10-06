import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fetchProtectedThumbnail } from "./thumbnails";
import { setUnauthorizedHandler, TOKEN_STORAGE_KEY } from "./client";

beforeEach(() => {
  localStorage.clear();
  vi.stubGlobal("fetch", vi.fn());
});
afterEach(() => {
  vi.unstubAllGlobals();
  setUnauthorizedHandler(null);
});
describe("protected thumbnail transport", () => {
  it("sends a bearer header to the same-origin thumbnail path without putting tokens in URLs", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "private-token");
    const blob = new Blob(["webp"], { type: "image/webp" });
    vi.mocked(fetch).mockResolvedValue(new Response(blob));
    expect(await fetchProtectedThumbnail("/t/key.webp")).toBeInstanceOf(Blob);
    const [url, init] = vi.mocked(fetch).mock.calls[0]!;
    expect(url).toBe("/t/key.webp");
    expect(new Headers(init?.headers).get("Authorization")).toBe("Bearer private-token");
  });
  it.each([
    "https://other.example/t/a.webp",
    "//other.example/t/a.webp",
    "/api/images",
    "/t/../api/private",
    "/t/a.webp?token=x",
    "/t/%2e%2e/api/private",
    "/t/%252e%252e/api/private",
    "/t/key%2f..%2fapi.webp",
    "/t/key%5c..%5capi.webp",
    "/t/\\..\\api/private",
    "javascript:alert(1)",
    "data:image/png;base64,eA==",
  ])("rejects unsafe path %s without a request", async (path) => {
    await expect(fetchProtectedThumbnail(path)).rejects.toThrow();
    expect(fetch).not.toHaveBeenCalled();
  });
  it("reports an expired session and never returns an error body as an image", async () => {
    const unauthorized = vi.fn();
    setUnauthorizedHandler(unauthorized);
    vi.mocked(fetch).mockResolvedValue(
      new Response(JSON.stringify({ code: 20001, message: "expired" }), { status: 401 }),
    );
    await expect(fetchProtectedThumbnail("/t/a.webp")).rejects.toMatchObject({
      code: 20001,
      status: 401,
    });
    expect(unauthorized).toHaveBeenCalledTimes(1);
  });

  it("does not clear a newly logged-in account after an old thumbnail request returns 401", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "old-session");
    const unauthorized = vi.fn(() => localStorage.removeItem(TOKEN_STORAGE_KEY));
    setUnauthorizedHandler(unauthorized);
    let respond!: (response: Response) => void;
    vi.mocked(fetch).mockImplementation(
      () =>
        new Promise((resolve) => {
          respond = resolve;
        }),
    );
    const promise = fetchProtectedThumbnail("/t/old.webp");
    localStorage.setItem(TOKEN_STORAGE_KEY, "new-session");
    respond(new Response(JSON.stringify({ code: 20001 }), { status: 401 }));
    await expect(promise).rejects.toMatchObject({ code: 20001 });
    expect(unauthorized).not.toHaveBeenCalled();
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBe("new-session");
  });

  it("does not clear a session when thumbnail error parsing completes after abort", async () => {
    const controller = new AbortController();
    const unauthorized = vi.fn();
    setUnauthorizedHandler(unauthorized);
    let resolveBody!: (body: unknown) => void;
    const response = new Response(null, { status: 401 });
    vi.spyOn(response, "json").mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveBody = resolve;
        }),
    );
    vi.mocked(fetch).mockResolvedValue(response);
    const promise = fetchProtectedThumbnail("/t/a.webp", controller.signal);
    await Promise.resolve();
    controller.abort();
    resolveBody({ code: 20001 });
    await expect(promise).rejects.toBeDefined();
    expect(unauthorized).not.toHaveBeenCalled();
  });
});
