import { createPinia } from "pinia";
import { createApp, watch } from "vue";
import { $t, i18n } from "@vben/locales";
import { setupAppI18n } from "./locales";
import { updatePreferences } from "@vben/preferences";
import App from "./App.vue";
import { setUnauthorizedHandler } from "./api/client";
import { connectVbenSession } from "./integrations/vben/session";
import { initShell } from "./integrations/shell/useShell";
import { enforceImgnestPreferences } from "./integrations/vben/preferences";
import { setupRouter } from "./router";
import { useAuthStore } from "./stores/auth";
import { useSiteStore } from "./stores/site";

export async function bootstrap(): Promise<void> {
  enforceImgnestPreferences();
  initShell();
  const app = createApp(App);
  const pinia = createPinia();
  app.use(pinia);
  await setupAppI18n(app);
  const router = setupRouter();
  const auth = useAuthStore(pinia);
  const site = useSiteStore(pinia);
  connectVbenSession(pinia, router);
  await auth.restore();
  app.use(router);

  setUnauthorizedHandler(() => {
    auth.clear();
    void router.push("/login").catch(() => {});
  });

  watch(
    () => site.siteName,
    (name) => updatePreferences({ app: { name } }),
    { immediate: true },
  );
  watch(
    () => [router.currentRoute.value.meta.title, site.siteName, i18n.global.locale.value],
    () => {
      document.title = `${$t(String(router.currentRoute.value.meta.title ?? "common.nav.upload"))} - ${site.siteName}`;
    },
    { immediate: true },
  );
  app.mount("#app");
  void site.ensureLoaded();
}
