import { createPinia } from "pinia";
import { createApp, watch } from "vue";
import { $t, i18n } from "@vben/locales";
import App from "./App.vue";
import { appConfig } from "./app.config";
import { setUnauthorizedHandler } from "./api/client";
import { hideAppLoading } from "./lib/appLoading";
import { setupAppI18n } from "./locales";
import { setupRouter } from "./router";
import { initLocaleMode } from "./shell/useLocaleMode";
import { enforcePreferences } from "./shell/preferences";
import { connectVbenSession } from "./shell/session";
import { loadSite, site } from "./shell/site";
import { initShell } from "./shell/useShell";
import { useAuthStore } from "./stores/auth";

export async function bootstrap(): Promise<void> {
  enforcePreferences();
  initShell();
  initLocaleMode();

  const app = createApp(App);
  const pinia = createPinia();
  app.use(pinia);
  await setupAppI18n(app);

  const router = setupRouter();
  const auth = useAuthStore(pinia);
  connectVbenSession(pinia, router);
  app.use(router);

  setUnauthorizedHandler(() => {
    auth.clear();
    void router.push(appConfig.loginPath).catch(() => {});
  });

  // 运行时站点信息（app.hooks.ts 的 loadSite），最多等 3 秒，失败用 app.config.ts 的默认值。
  await loadSite();

  watch(
    () => [router.currentRoute.value.meta.title, i18n.global.locale.value, site.title],
    () => {
      const title = router.currentRoute.value.meta.title;
      document.title = title ? `${$t(title)} - ${site.title}` : site.title;
    },
    { immediate: true },
  );

  app.mount("#app");
  // 首次导航（含登录守卫里的 /me）完成、页面组件渲染出来后再撤掉加载画面，避免中间露出空白。
  await router.isReady().catch(() => {});
  requestAnimationFrame(() => hideAppLoading());
}
