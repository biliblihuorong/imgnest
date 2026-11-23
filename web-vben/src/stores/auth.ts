import { defineStore } from "pinia";
import { onScopeDispose, ref, shallowRef, watch } from "vue";
import * as authApi from "@/api/auth";
import type { CurrentUser, LoginCredentials, LoginResult } from "@/api/types";
import { getSessionToken, setSessionToken, subscribeSession } from "@/lib/session";

function cancelledLogin(): DOMException {
  return new DOMException("Authentication attempt cancelled", "AbortError");
}

/**
 * 唯一的登录态来源。布局、菜单、路由守卫都只读这里；
 * Vben 自带的 access/user store 由 shell/session.ts 单向投影，不要直接写它们。
 *
 * 会话规则：
 * - token 与 lib/session.ts 双向同步；其他标签页登录/退出/换账号时，这里撤销旧会话并重新拉取用户。
 * - 新登录会取消进行中的旧登录和 /me 请求；迟到的结果不能重建已清除或已切换的会话。
 * - 其他登录方式（单点登录回调、扫码等）用 establish 或 setSession，不要往这里加登录流程。
 */
export const useAuthStore = defineStore("auth", () => {
  const token = shallowRef(getSessionToken());
  const user = ref<CurrentUser | null>(null);
  /** 每次登录/退出/换账号加一；App.vue 用它重建布局，避免上一个账号的页面状态残留。 */
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

  /** 把 token 写进 store 但不回写存储（值来自存储本身或马上要写）。 */
  function project(next: string | null): void {
    projecting = true;
    token.value = next;
    projecting = false;
  }

  // 其他标签页改了会话：撤销当前会话；有新 token 就重新拉取用户。
  const unsubscribe = subscribeSession((next) => {
    if (next === token.value && next !== null) return;
    invalidate();
    project(next);
    if (next) void restore();
  });

  // 直接给 token 赋值（测试或特殊流程）时同样撤销旧会话并持久化。
  watch(
    token,
    (next) => {
      if (projecting) return;
      invalidate();
      setSessionToken(next);
    },
    { flush: "sync" },
  );

  /**
   * 建立新会话：fetchSession 拿到 {token, user} 后写入。新调用会取消旧的登录与恢复请求；
   * 被取消或已过期时抛 AbortError。signal 用于调用方（例如离开登录页）主动放弃。
   */
  async function establish(
    signal: AbortSignal | undefined,
    fetchSession: (signal: AbortSignal) => Promise<LoginResult>,
  ): Promise<CurrentUser> {
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
      project(data.token);
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

  /** 账号密码登录；凭据形状由项目在 api/types.ts 的 LoginCredentials 定义。 */
  function login(credentials: LoginCredentials, signal?: AbortSignal): Promise<CurrentUser> {
    return establish(signal, (abort) => authApi.login(credentials, abort));
  }

  /**
   * 其他登录方式已经拿到 token 和用户时调用，效果和 login 成功一样。
   * 需要发请求换取会话的流程（单点登录票据等）用 establish，才能被新登录取消。
   */
  function setSession(nextToken: string, nextUser: CurrentUser): void {
    invalidate();
    project(nextToken);
    setSessionToken(nextToken);
    user.value = nextUser;
  }

  /** 有 token 但还没拿到用户信息时（刷新页面后）拉一次 /me；同一会话的并发调用共用同一个请求。 */
  function restore(): Promise<void> {
    if (!token.value || user.value) return Promise.resolve();
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
        const restoredUser = await authApi.me(controller.signal);
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

  /** 资料改了以后更新当前用户（只接受同一个账号）。 */
  function setUser(next: CurrentUser): void {
    if (token.value && user.value?.id === next.id) user.value = next;
  }

  /** 本地立即清空；服务端退出请求失败也不影响。 */
  async function logout(): Promise<void> {
    // 先发请求（同步带上旧 token）再清本地，迟到的响应不会影响之后的新会话。
    const request = token.value ? authApi.logout() : Promise.resolve(null);
    clear();
    try {
      await request;
    } catch {
      // 服务端吊销失败不阻塞本地退出。
    }
  }

  function clear(): void {
    invalidate();
    project(null);
    // 即使存储写入失败或本来就是匿名，也通知页面清理私有数据。
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
    establish,
    setSession,
    restore,
    setUser,
    logout,
    clear,
  };
});
