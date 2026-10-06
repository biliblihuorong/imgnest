import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, TOKEN_STORAGE_KEY } from "@/api/client";
import * as authApi from "@/api/auth";
import type { LoginData, UserView } from "@/api/types";
import { useAuthStore } from "./auth";

vi.mock("@/api/auth", () => ({
  login: vi.fn(),
  register: vi.fn(),
  logout: vi.fn(),
  me: vi.fn(),
  changePassword: vi.fn(),
}));

const authApiMock = vi.mocked(authApi);

const userFixture: UserView = {
  id: 1,
  group_id: 1,
  username: "alice",
  email: "alice@example.com",
  role: "user",
  status: "enabled",
  used_bytes: 0,
  created_at: "2026-10-04T00:00:00Z",
  display_name: "",
  avatar_provider: "weavatar",
  avatar_url: "https://weavatar.com/avatar/abc?s=160&d=404",
  avatar_config_version: 0,
};

describe("auth store", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.resetAllMocks();
    setActivePinia(createPinia());
  });

  it("login 成功后保存 token 与 user 并持久化到 localStorage", async () => {
    authApiMock.login.mockResolvedValue({
      token: "7|token",
      user: userFixture,
      expires_at: "2026-10-05T00:00:00Z",
    });
    const store = useAuthStore();

    await store.login("alice@example.com", "password-123");

    expect(store.token).toBe("7|token");
    expect(store.user).toEqual(userFixture);
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBe("7|token");
    expect(authApiMock.login).toHaveBeenCalledWith(
      "alice@example.com",
      "password-123",
      undefined,
      expect.any(AbortSignal),
    );
  });

  it("restore 时有效 token 恢复用户信息并保留 token", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|token");
    authApiMock.me.mockResolvedValue(userFixture);
    const store = useAuthStore();

    await store.restore();

    expect(store.token).toBe("7|token");
    expect(store.user).toEqual(userFixture);
    expect(authApiMock.me).toHaveBeenCalledTimes(1);
  });

  it("restore 时无效 token 清空全部状态", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|expired");
    authApiMock.me.mockRejectedValue(new ApiError(20001, "未登录", 401));
    const store = useAuthStore();

    await store.restore();

    expect(store.token).toBeNull();
    expect(store.user).toBeNull();
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBeNull();
  });

  it("restore 时无 token 则不发起请求", async () => {
    const store = useAuthStore();

    await store.restore();

    expect(authApiMock.me).not.toHaveBeenCalled();
    expect(store.user).toBeNull();
  });

  it("logout 调用接口并清空本地状态", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|token");
    authApiMock.logout.mockResolvedValue(null);
    const store = useAuthStore();
    store.user = userFixture;

    await store.logout();

    expect(authApiMock.logout).toHaveBeenCalledTimes(1);
    expect(store.token).toBeNull();
    expect(store.user).toBeNull();
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBeNull();
  });

  it("logout 接口失败时仍然清空本地状态", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|token");
    authApiMock.logout.mockRejectedValue(new ApiError(50001, "服务器错误", 500));
    const store = useAuthStore();
    store.user = userFixture;

    await store.logout();

    expect(store.token).toBeNull();
    expect(store.user).toBeNull();
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBeNull();
  });
  it("清除会话立即通知敏感预览清理且不携带数据", () => {
    const listener = vi.fn();
    window.addEventListener("imgnest:session-cleared", listener);
    const store = useAuthStore();
    store.clear();
    expect(listener).toHaveBeenCalledTimes(1);
    expect(listener.mock.calls[0]?.[0]).not.toHaveProperty("detail");
    window.removeEventListener("imgnest:session-cleared", listener);
  });
  it("forwards CAPTCHA only to login without persisting the challenge", async () => {
    authApiMock.login.mockResolvedValue({
      token: "7|token",
      user: userFixture,
      expires_at: "2026-10-05T00:00:00Z",
    });
    const store = useAuthStore();
    await store.login("alice@example.com", "password-123", "transient-challenge");
    expect(authApiMock.login).toHaveBeenCalledWith(
      "alice@example.com",
      "password-123",
      "transient-challenge",
      expect.any(AbortSignal),
    );
    expect(JSON.stringify(store.$state)).not.toContain("transient-challenge");
    expect(localStorage.length).toBe(1);
  });

  it("keeps the newest successful identity when an earlier login completes last", async () => {
    let finishFirst!: (value: LoginData) => void;
    authApiMock.login.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finishFirst = resolve;
        }),
    );
    const store = useAuthStore();
    const first = store.login("alice@example.com", "password-123").catch((error: unknown) => error);
    const secondUser = {
      ...userFixture,
      id: 2,
      username: "bob",
      email: "bob@example.com",
      role: "admin" as const,
    };
    authApiMock.login.mockResolvedValueOnce({
      token: "8|second",
      user: secondUser,
      expires_at: "2026-10-05T00:00:00Z",
    });
    await store.login("bob@example.com", "password-456");
    finishFirst({ token: "7|first", user: userFixture, expires_at: "2026-10-05T00:00:00Z" });
    const firstResult = await first;
    expect(store.token).toBe("8|second");
    expect(store.user).toEqual(secondUser);
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBe("8|second");
    expect(firstResult).toMatchObject({ name: "AbortError" });
    expect(authApiMock.login.mock.calls[0]?.[3]?.aborted).toBe(true);
  });

  it("does not recreate a cleared session when a pending login completes", async () => {
    let finish!: (value: LoginData) => void;
    authApiMock.login.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const store = useAuthStore();
    const result = store
      .login("alice@example.com", "password-123")
      .catch((error: unknown) => error);
    store.clear();
    finish({ token: "7|late", user: userFixture, expires_at: "2026-10-05T00:00:00Z" });
    await result;
    expect(store.token).toBeNull();
    expect(store.user).toBeNull();
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBeNull();
    expect(authApiMock.login.mock.calls[0]?.[3]?.aborted).toBe(true);
  });

  it("does not let completion of an old logout clear a newer login", async () => {
    let finishLogout!: (value: null) => void;
    authApiMock.logout.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finishLogout = resolve;
        }),
    );
    const store = useAuthStore();
    store.token = "7|old";
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|old");
    const logout = store.logout();
    authApiMock.login.mockResolvedValueOnce({
      token: "8|new",
      user: userFixture,
      expires_at: "2026-10-05T00:00:00Z",
    });
    await store.login("alice@example.com", "password-123");
    finishLogout(null);
    await logout;
    expect(store.token).toBe("8|new");
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBe("8|new");
  });
});
