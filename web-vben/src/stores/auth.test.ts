import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, getToken, request, TOKEN_STORAGE_KEY } from "@/api/client";
import { flushPromises } from "@vue/test-utils";
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

  it("localStorage 写入失败（隐私模式/配额满）时 login 仍成功，降级为仅内存会话", async () => {
    const setItem = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("The quota has been exceeded.", "QuotaExceededError");
    });
    try {
      authApiMock.login.mockResolvedValue({
        token: "7|token",
        user: userFixture,
        expires_at: "2026-10-05T00:00:00Z",
      });
      const store = useAuthStore();

      await expect(store.login("alice@example.com", "password-123")).resolves.toEqual(userFixture);

      expect(store.token).toBe("7|token");
      expect(store.user).toEqual(userFixture);
      expect(getToken()).toBe("7|token");
      expect(setItem).toHaveBeenCalledWith(TOKEN_STORAGE_KEY, "7|token");
    } finally {
      setItem.mockRestore();
    }
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

  it("并发 restore 只请求一次 /api/auth/me", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|token");
    authApiMock.me.mockResolvedValue(userFixture);
    const store = useAuthStore();

    await Promise.all([store.restore(), store.restore()]);

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
  it("storage 删除失败仍清理所有会话来源并通知敏感预览", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|token");
    const store = useAuthStore();
    store.user = userFixture;
    const listener = vi.fn();
    window.addEventListener("imgnest:session-cleared", listener);
    const remove = vi.spyOn(Storage.prototype, "removeItem").mockImplementation(() => {
      throw new DOMException("denied", "SecurityError");
    });
    try {
      expect(() => store.clear()).not.toThrow();
      expect(store.token).toBeNull();
      expect(store.user).toBeNull();
      expect(getToken()).toBeNull();
      expect(listener).toHaveBeenCalledTimes(1);
    } finally {
      remove.mockRestore();
      window.removeEventListener("imgnest:session-cleared", listener);
    }
  });
  it("跨标签切换立即丢弃旧身份和预览，用新 Token 恢复真实用户", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|alice");
    const store = useAuthStore();
    store.user = userFixture;
    const bob = { ...userFixture, id: 2, username: "bob" };
    let finish!: (value: UserView) => void;
    authApiMock.me.mockReturnValue(
      new Promise((resolve) => {
        finish = resolve;
      }),
    );
    const listener = vi.fn();
    window.addEventListener("imgnest:session-cleared", listener);
    try {
      localStorage.setItem(TOKEN_STORAGE_KEY, "8|bob");
      window.dispatchEvent(
        new StorageEvent("storage", {
          key: TOKEN_STORAGE_KEY,
          oldValue: "7|alice",
          newValue: "8|bob",
          storageArea: localStorage,
        }),
      );
      expect(store.token).toBe("8|bob");
      expect(store.user).toBeNull();
      expect(listener).toHaveBeenCalledTimes(1);
      finish(bob);
      await flushPromises();
      expect(store.user).toEqual(bob);
      const fetchMock = vi
        .fn()
        .mockResolvedValue(new Response(JSON.stringify({ code: 0, data: null })));
      vi.stubGlobal("fetch", fetchMock);
      await request("/api/images");
      expect((fetchMock.mock.calls[0]![1].headers as Headers).get("Authorization")).toBe(
        "Bearer 8|bob",
      );
    } finally {
      vi.unstubAllGlobals();
      window.removeEventListener("imgnest:session-cleared", listener);
    }
  });
  it("跨标签清空 storage 撤销未完成登录", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|alice");
    const store = useAuthStore();
    let finish!: (value: LoginData) => void;
    authApiMock.login.mockReturnValue(
      new Promise((resolve) => {
        finish = resolve;
      }),
    );
    const login = store.login("alice@example.com", "password-123").catch((error: unknown) => error);
    localStorage.clear();
    window.dispatchEvent(new StorageEvent("storage", { key: null, storageArea: localStorage }));
    finish({ token: "7|late", user: userFixture, expires_at: "2026-10-05T00:00:00Z" });
    expect(await login).toMatchObject({ name: "AbortError" });
    expect(store.token).toBeNull();
    expect(store.user).toBeNull();
  });
  it("迟到的 storage.clear 事件不撤销已经建立的新会话", async () => {
    authApiMock.login.mockResolvedValue({
      token: "7|new",
      user: userFixture,
      expires_at: "2026-10-14T00:00:00Z",
    });
    const store = useAuthStore();
    await store.login("alice@example.com", "password-123");
    const cleared = vi.fn();
    window.addEventListener("imgnest:session-cleared", cleared);
    try {
      window.dispatchEvent(new StorageEvent("storage", { key: null, storageArea: localStorage }));
      expect(cleared).not.toHaveBeenCalled();
      expect(store.token).toBe("7|new");
      expect(store.user).toEqual(userFixture);
    } finally {
      window.removeEventListener("imgnest:session-cleared", cleared);
    }
  });
  it("storage 事件送达前不把旧账号的动作改用另一账号的 Token", async () => {
    authApiMock.login.mockResolvedValue({
      token: "7|alice",
      user: userFixture,
      expires_at: "2026-10-14T00:00:00Z",
    });
    const store = useAuthStore();
    await store.login("alice@example.com", "password-123");
    const bob = { ...userFixture, id: 2, username: "bob" };
    authApiMock.me.mockResolvedValue(bob);
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({ code: 0, data: null })));
    vi.stubGlobal("fetch", fetchMock);
    try {
      localStorage.setItem(TOKEN_STORAGE_KEY, "8|bob");
      await request("/api/auth/profile", {
        method: "PATCH",
        body: JSON.stringify({ display_name: "Alice" }),
      });
      expect((fetchMock.mock.calls[0]![1].headers as Headers).get("Authorization")).toBe(
        "Bearer 7|alice",
      );
      expect(store.user).toEqual(userFixture);
      window.dispatchEvent(
        new StorageEvent("storage", {
          key: TOKEN_STORAGE_KEY,
          oldValue: "7|alice",
          newValue: "8|bob",
          storageArea: localStorage,
        }),
      );
      await store.restore();
      expect(store.user).toEqual(bob);
      expect(getToken()).toBe("8|bob");
    } finally {
      vi.unstubAllGlobals();
    }
  });
  it("旧 restore 完成后不会阻止新会话 restore 或覆盖新身份", async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, "7|alice");
    let finishOld!: (value: UserView) => void;
    authApiMock.me.mockReturnValueOnce(
      new Promise((resolve) => {
        finishOld = resolve;
      }),
    );
    const store = useAuthStore();
    const oldRestore = store.restore();
    store.clear();
    const bob = { ...userFixture, id: 2, username: "bob" };
    authApiMock.me.mockResolvedValueOnce(bob);
    localStorage.setItem(TOKEN_STORAGE_KEY, "8|bob");
    window.dispatchEvent(
      new StorageEvent("storage", {
        key: TOKEN_STORAGE_KEY,
        oldValue: null,
        newValue: "8|bob",
        storageArea: localStorage,
      }),
    );
    await store.restore();
    expect(store.user).toEqual(bob);
    finishOld(userFixture);
    await oldRestore;
    expect(store.user).toEqual(bob);
    expect(authApiMock.me).toHaveBeenCalledTimes(2);
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
