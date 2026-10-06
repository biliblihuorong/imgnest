import type { Pinia } from "pinia";
import type { Router } from "vue-router";
import type { MenuRecordRaw } from "@vben/types";
import { useAccessStore, useTabbarStore, useUserStore } from "@vben/stores";
import { computed, markRaw, watch } from "vue";
import { $t, i18n } from "@vben/locales";
import { useSiteStore } from "@/stores/site";
import { landingPath } from "@/router/landing";
import { useAuthStore } from "@/stores/auth";
import { useUserAvatar } from "@/components/account/useUserAvatar";
import { menuIcons } from "./menuIcons";
import { workspaceMenus, type WorkspaceMenu } from "./navigation";

function menuWithIcons(menu: WorkspaceMenu): MenuRecordRaw {
  const icon = menuIcons[menu.icon];
  return {
    name: $t(menu.name),
    path: menu.path,
    icon: icon ? markRaw(icon) : undefined,
    children: menu.children?.map(menuWithIcons),
  };
}

/**
 * ImageNest auth 是唯一认证来源；Vben stores 仅投影用户/菜单/标签页 UI。
 * 不复制 Token，不启用模板的刷新 Token/动态菜单/权限码后端。
 */
export function connectVbenSession(pinia: Pinia, router: Router): () => void {
  const auth = useAuthStore(pinia);
  const site = useSiteStore(pinia);
  const access = useAccessStore(pinia);
  const userStore = useUserStore(pinia);
  const tabbar = useTabbarStore(pinia);
  // 与 AppLayout 共用同一头像数据源：本地占位 → 远程加载成功后替换。
  const avatarUrl = useUserAvatar(computed(() => auth.user));
  let previousIdentity: string | undefined;

  return watch(
    () =>
      [
        auth.user,
        Boolean(auth.token),
        site.galleryEnabled,
        i18n.global.locale.value,
        avatarUrl.value,
      ] as const,
    ([user, hasToken, galleryEnabled, , avatar]) => {
      const current = hasToken ? user : null;
      const identity = current ? `${current.id}:${current.role}` : "anonymous";
      if (previousIdentity !== undefined && previousIdentity !== identity) {
        tabbar.$reset();
        const activeRoute = router.currentRoute.value;
        if (
          !current &&
          activeRoute.matched.length > 0 &&
          !activeRoute.meta.bare &&
          activeRoute.path !== "/gallery"
        ) {
          void router.replace("/login").catch(() => {});
        }
        // Static routes stay installed; leave a privileged page immediately when
        // the restored account loses access, so its local data is disposed too.
        if (
          current &&
          current.role !== "admin" &&
          router.currentRoute.value.matched.some((record) => record.meta.admin)
        ) {
          void router.replace("/forbidden").catch(() => {});
        }
      }
      previousIdentity = identity;
      access.setAccessToken(null);
      access.setRefreshToken(null);
      access.setAccessCodes([]);
      access.setAccessMenus(workspaceMenus(current?.role, galleryEnabled).map(menuWithIcons));
      access.setAccessRoutes(
        current
          ? router.options.routes.filter(
              (route) => !route.meta?.bare && (current.role === "admin" || !route.meta?.admin),
            )
          : [],
      );
      access.setIsAccessChecked(Boolean(current));
      userStore.setUserInfo(
        current
          ? {
              userId: String(current.id),
              username: current.username,
              realName: current.display_name || current.username,
              avatar,
              roles: [current.role],
              homePath: landingPath(current.role),
            }
          : null,
      );
    },
    { immediate: true, flush: "sync", deep: true },
  );
}
