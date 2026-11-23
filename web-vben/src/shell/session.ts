import type { Pinia } from "pinia";
import type { Router } from "vue-router";
import { useAccessStore, useTabbarStore, useUserStore } from "@vben/stores";
import { computed, watch } from "vue";
import { i18n } from "@vben/locales";
import { appConfig } from "@/app.config";
import { canAccess, homePath, onIdentityChange, useAvatar } from "@/app.hooks";
import { useAuthStore } from "@/stores/auth";
import { allowedFor, isOpenRoute } from "@/router";
import { shellMenus, toVbenMenu } from "./navigation";

/**
 * 把 auth store 单向投影到 Vben 的 access/user/tabbar store，经典布局靠它们渲染菜单和头像。
 * 不复制 Token，不启用 Vben 的刷新 Token、动态菜单与权限码。
 * 菜单来自 app.hooks.ts 的 resolveMenus，它读到的状态变化时这里会重新投影。
 */
export function connectVbenSession(pinia: Pinia, router: Router): () => void {
  const auth = useAuthStore(pinia);
  const access = useAccessStore(pinia);
  const userStore = useUserStore(pinia);
  const tabbar = useTabbarStore(pinia);
  const signedIn = computed(() => (auth.token ? auth.user : null));
  const avatar = useAvatar(signedIn);
  let previousIdentity: string | undefined;

  return watch(
    () => {
      const current = signedIn.value;
      return [
        auth.user,
        Boolean(auth.token),
        i18n.global.locale.value,
        avatar.value,
        // 在这里求值，resolveMenus 读到的 store 状态才会被追踪。
        shellMenus(current).map(toVbenMenu),
      ] as const;
    },
    ([user, hasToken, , avatarUrl, menus]) => {
      const current = hasToken ? user : null;
      // 有 token 但用户信息还在加载（刷新页面后）不算换身份，否则刚打开的标签页会被清掉。
      const pending = hasToken && !user;
      const identity = current ? `${current.id}:${current.role}` : "anonymous";
      if (!pending && previousIdentity !== identity) {
        if (previousIdentity !== undefined) {
          tabbar.$reset();
          const active = router.currentRoute.value;
          const onPrivatePage = active.matched.length > 0 && !isOpenRoute(active);
          // 退出或换账号后立刻离开没权限的页面，页面里的数据随组件一起销毁。
          if (!current && onPrivatePage) void router.replace(appConfig.loginPath).catch(() => {});
          else if (current && active.matched.length > 0 && !allowedFor(active, current))
            void router.replace("/forbidden").catch(() => {});
        }
        previousIdentity = identity;
        onIdentityChange(current);
      }

      access.setAccessToken(null);
      access.setRefreshToken(null);
      access.setAccessCodes([]);
      access.setAccessMenus(menus);
      access.setAccessRoutes(
        current
          ? router.options.routes.filter(
              (route) => !route.meta?.bare && canAccess(current, route.meta ?? {}),
            )
          : [],
      );
      access.setIsAccessChecked(Boolean(current));
      userStore.setUserInfo(
        current
          ? {
              userId: String(current.id),
              username: current.username,
              realName: current.displayName || current.username,
              avatar: avatarUrl,
              roles: [current.role],
              homePath: homePath(current),
            }
          : null,
      );
    },
    { immediate: true, flush: "sync", deep: true },
  );
}
