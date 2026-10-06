import { shallowReadonly, shallowRef } from "vue";
import { useRouter } from "vue-router";
import { useAuthStore } from "@/stores/auth";

/** 两套外壳共用的退出登录：防重入，先离开敏感页面再等待请求。 */
export function useLogout() {
  const router = useRouter();
  const auth = useAuthStore();
  const loggingOut = shallowRef(false);

  async function logout(): Promise<void> {
    if (loggingOut.value) return;
    loggingOut.value = true;
    try {
      const request = auth.logout();
      // Leave the sensitive view immediately. A delayed logout response must never
      // navigate a newer session away from its current workspace.
      await router.replace("/login");
      await request;
    } finally {
      loggingOut.value = false;
    }
  }

  return { loggingOut: shallowReadonly(loggingOut), logout };
}
