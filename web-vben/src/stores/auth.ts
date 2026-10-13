import { defineStore } from "pinia";
import { onScopeDispose, ref, shallowRef, watch } from "vue";
import { exchangeSsoTicket, login as loginApi, logout as logoutApi, me as meApi } from "@/api/auth";
import type { LoginData, UserView } from "@/api/types";
import { getSessionToken, setSessionToken, subscribeSession } from "@/lib/session";

function cancelledLogin(): DOMException {
  return new DOMException("Authentication attempt cancelled", "AbortError");
}

export const useAuthStore = defineStore("auth", () => {
  const token = shallowRef(getSessionToken());
  const user = ref<UserView | null>(null);
  const sessionGeneration = shallowRef(0);
  let pendingLogin: AbortController | undefined;
  let pendingRestore: { controller: AbortController; promise: Promise<void> } | undefined;
  let projecting = false;

  function cancelRestore(): void {
    pendingRestore?.controller.abort();
    pendingRestore = undefined;
  }

  function finishRestore(controller: AbortController): void {
    if (pendingRestore?.controller === controller) pendingRestore = undefined;
  }

  function invalidate(): void {
    sessionGeneration.value++;
    pendingLogin?.abort();
    pendingLogin = undefined;
    cancelRestore();
    user.value = null;
  }

  const unsubscribe = subscribeSession((next) => {
    if (next === token.value && next !== null) return;
    invalidate();
    projecting = true;
    token.value = next;
    projecting = false;
    if (next) void restore();
  });

  // Pinia 状态也是 session 的同步投影，所有写入都经同一持久化/内存降级入口。
  watch(
    token,
    (next) => {
      if (projecting) return;
      invalidate();
      setSessionToken(next);
    },
    { flush: "sync" },
  );

  function login(
    email: string,
    password: string,
    captchaToken?: string,
    signal?: AbortSignal,
  ): Promise<UserView> {
    return establish(signal, (abort) => loginApi(email, password, captchaToken, abort));
  }

  function loginWithSsoTicket(ticket: string, signal?: AbortSignal): Promise<UserView> {
    return establish(signal, (abort) => exchangeSsoTicket(ticket, abort));
  }

  /** 新登录取消旧请求；过期结果不能重建已清除或已切换的会话。 */
  async function establish(
    signal: AbortSignal | undefined,
    fetchSession: (signal: AbortSignal) => Promise<LoginData>,
  ): Promise<UserView> {
    const generation = ++sessionGeneration.value;
    pendingLogin?.abort();
    cancelRestore();
    const controller = new AbortController();
    pendingLogin = controller;
    const abort = () => controller.abort();
    signal?.addEventListener("abort", abort, { once: true });
    if (signal?.aborted) abort();
    const current = () => !controller.signal.aborted && sessionGeneration.value === generation;
    try {
      if (!current()) throw cancelledLogin();
      const data = await fetchSession(controller.signal);
      if (!current()) throw cancelledLogin();
      projecting = true;
      token.value = data.token;
      projecting = false;
      setSessionToken(data.token);
      user.value = data.user;
      return data.user;
    } catch (error) {
      if (!current()) throw cancelledLogin();
      throw error;
    } finally {
      signal?.removeEventListener("abort", abort);
      if (pendingLogin === controller) pendingLogin = undefined;
    }
  }

  async function logout(): Promise<void> {
    // API 同步捕获旧 Bearer 后清态，迟到的退出响应不能影响新会话。
    const request = logoutApi();
    clear();
    try {
      await request;
    } catch {
      // 服务端吊销失败不阻塞本地退出。
    }
  }

  /** 同会话恢复合并请求；跨会话更换会撤销旧请求和去重记录。 */
  function restore(): Promise<void> {
    if (!token.value) return Promise.resolve();
    if (pendingRestore) return pendingRestore.promise;
    const restoredToken = token.value;
    const generation = sessionGeneration.value;
    const controller = new AbortController();
    const current = () =>
      !controller.signal.aborted &&
      token.value === restoredToken &&
      sessionGeneration.value === generation;
    const promise = (async () => {
      try {
        const restoredUser = await meApi(controller.signal);
        if (current()) user.value = restoredUser;
      } catch {
        if (current()) clear();
      } finally {
        finishRestore(controller);
      }
    })();
    pendingRestore = { controller, promise };
    return promise;
  }

  function setUser(next: UserView): void {
    if (token.value && user.value?.id === next.id) user.value = next;
  }

  function clear(): void {
    invalidate();
    projecting = true;
    token.value = null;
    projecting = false;
    // 即使 removeItem 失败或原本已匿名，也通知敏感数据清理。
    setSessionToken(null, true);
  }

  onScopeDispose(() => {
    unsubscribe();
    pendingLogin?.abort();
    cancelRestore();
  });

  return {
    token,
    user,
    sessionGeneration,
    login,
    loginWithSsoTicket,
    establish,
    logout,
    restore,
    setUser,
    clear,
  };
});
