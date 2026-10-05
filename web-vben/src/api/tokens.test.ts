import { beforeEach, describe, expect, it, vi } from "vitest";
import { request } from "./client";
import { createToken, listTokens, revokeToken } from "./tokens";
import type { IssuedToken } from "./tokens";

vi.mock("./client", () => ({
  request: vi.fn(),
}));

const requestMock = vi.mocked(request);

const issuedFixture: IssuedToken = {
  token: "7|abcdefghijklmnopqrstuvwxyz0123456789ABCD",
  info: {
    id: 7,
    name: "blog-client",
    kind: "api",
    abilities: ["*"],
    expires_at: null,
    last_used_at: null,
    created_at: "2026-10-04T00:00:00Z",
  },
};

describe("api/tokens", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it("listTokens 以 GET 请求 /api/tokens 并返回数组", async () => {
    requestMock.mockResolvedValueOnce([issuedFixture.info]);

    await expect(listTokens()).resolves.toEqual([issuedFixture.info]);
    expect(requestMock).toHaveBeenCalledWith("/api/tokens");
  });

  it("createToken 以 POST 请求 /api/tokens，无过期时间时省略 expires_at", async () => {
    requestMock.mockResolvedValueOnce(issuedFixture);

    await expect(createToken({ name: "blog-client" })).resolves.toEqual(issuedFixture);
    expect(requestMock).toHaveBeenCalledWith("/api/tokens", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name: "blog-client" }),
    });
  });

  it("createToken 按原样传输 RFC3339 UTC 的 expires_at", async () => {
    requestMock.mockResolvedValueOnce(issuedFixture);

    await createToken({ name: "blog-client", expires_at: "2026-11-04T00:00:00Z" });
    expect(requestMock).toHaveBeenCalledWith("/api/tokens", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name: "blog-client", expires_at: "2026-11-04T00:00:00Z" }),
    });
  });

  it("createToken 显式 null 的 expires_at 按 null 传输", async () => {
    requestMock.mockResolvedValueOnce(issuedFixture);

    await createToken({ name: "blog-client", expires_at: null });
    expect(requestMock).toHaveBeenCalledWith("/api/tokens", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name: "blog-client", expires_at: null }),
    });
  });

  it("revokeToken 以 DELETE 请求 /api/tokens/{id}", async () => {
    requestMock.mockResolvedValueOnce(null);

    await expect(revokeToken(7)).resolves.toBeNull();
    expect(requestMock).toHaveBeenCalledWith("/api/tokens/7", { method: "DELETE" });
  });
});
