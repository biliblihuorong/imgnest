import { shallowReadonly, shallowRef } from "vue";
import { useRouter } from "vue-router";
import { appConfig } from "@/app.config";
import { useAuthStore } from "@/stores/auth";

/** 两套外壳共用的退出登录：防重入，先离开当前页面再等服务端响应。 */
export function useLogout() {
  const router = useRouter();
  const auth = useAuthStore();
  const loggingOut = shallowRef(false);

  async function logout(): Promise<void> {
    if (loggingOut.value) return;
    loggingOut.value = true;
    try {
      const request = auth.logout();
      await router.replace(appConfig.loginPath);
      await request;
    } finally {
      loggingOut.value = false;
    }
  }

  return { loggingOut: shallowReadonly(loggingOut), logout };
}
