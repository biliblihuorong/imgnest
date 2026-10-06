import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, request, setUnauthorizedHandler, TOKEN_STORAGE_KEY } from "./client";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function calledInit(mock: ReturnType<typeof vi.fn>): RequestInit {
  const init = mock.mock.calls[0]?.[1];
  expect(init).toBeDefined();
  return init as RequestInit;
}

describe("api client request", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(() => {
    setUnauthorizedHandler(null);
    vi.unstubAllGlobals();
  });

  it("code=0 时解包外壳并返回 data", async () => {
    const payload = { hello: "world" };
    const fetchMock = vi
      .fn()
      .mockResolvedValue(jsonResponse({ code: 0, message: "ok", data: payload }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(request<{ hello: string }>("/api/ping")).resolves.toEqual(payload);
    expect(fetchMock).toHaveBeenCalledWith("/api/ping", expect.anything());
  });

  it("存在 token 时自动附加 Bearer Authorization 头", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|abc");
    const fetchMock = vi
      .fn()
      .mockResolvedValue(jsonResponse({ code: 0, message: "ok", data: null }));
    vi.stubGlobal("fetch", fetchMock);

    await request<null>("/api/auth/me");
    expect(new Headers(calledInit(fetchMock).headers).get("Authorization")).toBe("Bearer 7|abc");
  });

  it("无 token 时不附加 Authorization 头", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(jsonResponse({ code: 0, message: "ok", data: null }));
    vi.stubGlobal("fetch", fetchMock);

    await request<null>("/api/site");
    expect(new Headers(calledInit(fetchMock).headers).get("Authorization")).toBeNull();
  });

  it("保留调用方传入的头并合并 Authorization", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|abc");
    const fetchMock = vi
      .fn()
      .mockResolvedValue(jsonResponse({ code: 0, message: "ok", data: null }));
    vi.stubGlobal("fetch", fetchMock);

    await request<null>("/api/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
    });
    const headers = new Headers(calledInit(fetchMock).headers);
    expect(headers.get("Content-Type")).toBe("application/json");
    expect(headers.get("Authorization")).toBe("Bearer 7|abc");
  });

  it("code!=0 时抛出 ApiError 且字段正确", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(jsonResponse({ code: 10001, message: "参数错误", data: null }, 400)),
    );

    const error = await request("/api/bad").catch((caught: unknown) => caught);
    expect(error).toBeInstanceOf(ApiError);
    const apiError = error as ApiError;
    expect(apiError.code).toBe(10001);
    expect(apiError.message).toBe("参数错误");
    expect(apiError.status).toBe(400);
  });

  it("HTTP 401 响应触发未授权回调", async () => {
    const handler = vi.fn();
    setUnauthorizedHandler(handler);
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(jsonResponse({ code: 20001, message: "未登录", data: null }, 401)),
    );

    await expect(request("/api/auth/me")).rejects.toBeInstanceOf(ApiError);
    expect(handler).toHaveBeenCalledTimes(1);
  });

  it("does not clear a new account when a previous account's 401 arrives late", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "old-session");
    const handler = vi.fn(() => localStorage.removeItem(TOKEN_STORAGE_KEY));
    setUnauthorizedHandler(handler);
    let respond!: (response: Response) => void;
    vi.stubGlobal(
      "fetch",
      vi.fn(
        () =>
          new Promise<Response>((resolve) => {
            respond = resolve;
          }),
      ),
    );
    const promise = request("/api/auth/me");
    localStorage.setItem(TOKEN_STORAGE_KEY, "new-session");
    respond(jsonResponse({ code: 20001, message: "expired", data: null }, 401));
    await expect(promise).rejects.toMatchObject({ code: 20001, status: 401 });
    expect(handler).not.toHaveBeenCalled();
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBe("new-session");
  });

  it("does not clear the session when an aborted request finishes parsing a 401", async () => {
    const controller = new AbortController();
    const handler = vi.fn();
    setUnauthorizedHandler(handler);
    let resolveBody!: (body: unknown) => void;
    const response = jsonResponse(null, 401);
    vi.spyOn(response, "json").mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveBody = resolve;
        }),
    );
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response));
    const promise = request("/api/auth/me", { signal: controller.signal });
    await Promise.resolve();
    controller.abort();
    resolveBody({ code: 20001, message: "expired", data: null });
    await expect(promise).rejects.toBeDefined();
    expect(handler).not.toHaveBeenCalled();
  });

  it("业务码 20002（凭证内容错误）不清会话，只抛错给调用方", async () => {
    const handler = vi.fn();
    setUnauthorizedHandler(handler);
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          jsonResponse({ code: 20002, message: "当前密码不正确", data: null }, 401),
        ),
    );

    await expect(request("/api/auth/password", { method: "PATCH" })).rejects.toBeInstanceOf(
      ApiError,
    );
    expect(handler).not.toHaveBeenCalled();
  });

  it("fetch 抛出异常时抛出 code=-1 的网络错误", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("Failed to fetch")));

    const error = await request("/api/x").catch((caught: unknown) => caught);
    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).code).toBe(-1);
    expect((error as ApiError).message).toBe("网络错误");
    expect((error as ApiError).status).toBe(0);
  });

  it("响应体不是合法 JSON 时按网络错误处理", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response("gateway timeout", { status: 504 })),
    );

    const error = await request("/api/x").catch((caught: unknown) => caught);
    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).code).toBe(-1);
    expect((error as ApiError).status).toBe(0);
  });
});

it("preserves versioned search diagnostics and UTF16 spans in ApiError", async () => {
  const data = {
    diagnostics: [
      {
        code: "UNKNOWN_FIELD",
        messageKey: "search.error.unknownField",
        span: { start: 3, end: 6 },
        args: { field: "foo" },
      },
    ],
  };
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(jsonResponse({ code: 10001, message: "query invalid", data }, 400)),
  );
  await expect(request("/api/images?qv=1")).rejects.toMatchObject({
    code: 10001,
    status: 400,
    data,
  });
  vi.unstubAllGlobals();
});
it("preserves abort identity rather than replacing cancellation with a network error", async () => {
  const abort = new DOMException("cancelled", "AbortError");
  vi.stubGlobal("fetch", vi.fn().mockRejectedValue(abort));
  await expect(request("/api/images", { signal: new AbortController().signal })).rejects.toBe(
    abort,
  );
  vi.unstubAllGlobals();
});
