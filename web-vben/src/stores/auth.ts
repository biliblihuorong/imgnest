import { defineStore } from "pinia";
import { login as loginApi, logout as logoutApi, me as meApi } from "@/api/auth";
import { TOKEN_STORAGE_KEY } from "@/api/client";
import type { UserView } from "@/api/types";

interface AuthState {
  token: string | null;
  user: UserView | null;
  sessionGeneration: number;
}

const pendingLogins = new WeakMap<object, AbortController>();
function cancelledLogin(): DOMException {
  return new DOMException("Authentication attempt cancelled", "AbortError");
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
    sessionGeneration: 0,
  }),
  actions: {
    /** 登录：保存 token/user 并持久化 token；失败抛 ApiError。 */
    async login(
      email: string,
      password: string,
      captchaToken?: string,
      signal?: AbortSignal,
    ): Promise<UserView> {
      const generation = ++this.sessionGeneration;
      pendingLogins.get(this)?.abort();
      const controller = new AbortController();
      pendingLogins.set(this, controller);
      const abort = () => controller.abort();
      signal?.addEventListener("abort", abort, { once: true });
      if (signal?.aborted) abort();
      const current = () => !controller.signal.aborted && this.sessionGeneration === generation;
      try {
        if (!current()) throw cancelledLogin();
        const data = await loginApi(email, password, captchaToken, controller.signal);
        if (!current()) throw cancelledLogin();
        this.token = data.token;
        this.user = data.user;
        localStorage.setItem(TOKEN_STORAGE_KEY, data.token);
        return data.user;
      } catch (error) {
        if (!current()) throw cancelledLogin();
        throw error;
      } finally {
        signal?.removeEventListener("abort", abort);
        if (pendingLogins.get(this) === controller) pendingLogins.delete(this);
      }
    },
    /** 登出：调用 API（失败忽略）并清空本地状态。 */
    async logout(): Promise<void> {
      // Capture/send the current bearer before clearing; an old response cannot clear a new login.
      const request = logoutApi();
      this.clear();
      try {
        await request;
      } catch {
        // 服务端吊销失败不阻塞本地登出
      }
    },
    /** 启动恢复：有 token 时用 me() 校验，失败清空状态。 */
    async restore(): Promise<void> {
      if (!this.token) {
        return;
      }
      const restoredToken = this.token;
      const generation = this.sessionGeneration;
      try {
        const user = await meApi();
        if (this.token === restoredToken && this.sessionGeneration === generation) this.user = user;
      } catch {
        if (this.token === restoredToken && this.sessionGeneration === generation) this.clear();
      }
    },
    /** 用服务端返回的最新用户视图覆盖当前会话用户；仅供本人资料更新使用。 */
    setUser(user: UserView): void {
      if (this.token && this.user?.id === user.id) {
        this.user = user;
      }
    },
    /** 清空登录态（401 回调与 logout 共用；不发起任何请求）。 */
    clear(): void {
      this.sessionGeneration++;
      pendingLogins.get(this)?.abort();
      pendingLogins.delete(this);
      this.token = null;
      this.user = null;
      localStorage.removeItem(TOKEN_STORAGE_KEY);
      window.dispatchEvent(new Event("imgnest:session-cleared"));
    },
  },
});
