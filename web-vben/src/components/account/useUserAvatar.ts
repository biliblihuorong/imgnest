import { getCurrentScope, onScopeDispose, shallowRef, watch, type Ref } from "vue";
import type { UserView } from "@/api/types";
import fallbackAvatar from "@/assets/account-avatar.svg";

/**
 * 远程头像加载上限。这是本项目的工程选择（外部 404/故障/慢响应都回退本地），
 * 不是供应商承诺；普通 img 的 error 无法区分 HTTP 状态码，也不假装能读到。
 */
export const AVATAR_LOAD_TIMEOUT_MS = 5000;

/**
 * 当前用户头像的统一数据源：先显示本地占位头像，远程图预加载成功且账户/URL
 * 仍然匹配时才替换。邮箱为空、404、网络失败、解码失败或超时都保持本地默认图；
 * 一次失败不循环重试，账户切换或组件销毁时取消旧请求，迟到成功不覆盖新头像。
 * AppLayout 顶部与 Vben session 投影共用本组合函数，保证两处显示一致。
 */
export function useUserAvatar(user: Ref<UserView | null | undefined>, size: 160 | 320 = 160) {
  const url = shallowRef<string>(fallbackAvatar);
  let generation = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;

  function stopPending() {
    generation += 1;
    if (timer !== undefined) {
      clearTimeout(timer);
      timer = undefined;
    }
  }

  watch(
    () => [user.value?.id, user.value?.avatar_url, user.value?.avatar_config_version] as const,
    ([id, remote]) => {
      stopPending();
      url.value = fallbackAvatar;
      if (!id || !remote) return;
      // 尺寸由代码受控（资料页 320，其余 160），不允许外部拼任意查询参数。
      const address = size === 320 ? remote.replace(/([?&])s=160\b/, "$1s=320") : remote;
      const probe = new Image();
      probe.referrerPolicy = "no-referrer";
      const current = generation;
      timer = setTimeout(() => {
        timer = undefined;
        probe.onload = null;
        probe.onerror = null;
        probe.src = "";
      }, AVATAR_LOAD_TIMEOUT_MS);
      probe.onload = () => {
        if (current === generation) {
          stopPending();
          url.value = address;
        }
      };
      probe.onerror = () => {
        // 保持本地默认图；不做第二次尝试。
        if (current === generation) stopPending();
      };
      probe.src = address;
    },
    { immediate: true },
  );

  // session 投影在组件外调用本组合函数；有作用域时才挂清理。
  if (getCurrentScope()) onScopeDispose(stopPending);
  return url;
}
