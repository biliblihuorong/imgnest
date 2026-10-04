import { defineStore } from "pinia";
import { login as loginApi, logout as logoutApi, me as meApi } from "@/api/auth";
import { TOKEN_STORAGE_KEY } from "@/api/client";
import type { UserView } from "@/api/types";

interface AuthState {
  token: string | null;
  user: UserView | null;
}

function readStoredToken(): string | null {
  try {
    return localStorage.getItem(TOKEN_STORAGE_KEY);
  } catch {
    return null;
  }
}

export const useAuthStore = defineStore("auth", {
  state: (): AuthState => ({
    token: readStoredToken(),
    user: null,
  }),
  actions: {
    /** 登录：保存 token/user 并持久化 token；失败抛 ApiError。 */
    async login(email: string, password: string): Promise<UserView> {
      const data = await loginApi(email, password);
      this.token = data.token;
      this.user = data.user;
      localStorage.setItem(TOKEN_STORAGE_KEY, data.token);
      return data.user;
    },
    /** 登出：调用 API（失败忽略）并清空本地状态。 */
    async logout(): Promise<void> {
      try {
        await logoutApi();
      } catch {
        // 服务端吊销失败不阻塞本地登出
      }
      this.clear();
    },
    /** 启动恢复：有 token 时用 me() 校验，失败清空状态。 */
    async restore(): Promise<void> {
      if (!this.token) {
        return;
      }
      try {
        this.user = await meApi();
      } catch {
        this.clear();
      }
    },
    /** 清空登录态（401 回调与 logout 共用；不发起任何请求）。 */
    clear(): void {
      this.token = null;
      this.user = null;
      localStorage.removeItem(TOKEN_STORAGE_KEY);
    },
  },
});
