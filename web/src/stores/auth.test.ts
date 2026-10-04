import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, TOKEN_STORAGE_KEY } from "@/api/client";
import * as authApi from "@/api/auth";
import type { UserView } from "@/api/types";
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
    expect(authApiMock.login).toHaveBeenCalledWith("alice@example.com", "password-123");
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
});
