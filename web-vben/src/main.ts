import { initPreferences, preferences } from "@vben/preferences";
import "@vben/styles";
import "@vben/styles/naive";
import { appConfig } from "./app.config";
import { showAppLoadError } from "./lib/appLoading";
import { appPreferences } from "./shell/preferences";
import "./assets/kit.css";
import "./assets/app.css";
import "./assets/marvis.css";

async function start(): Promise<void> {
  await initPreferences({
    namespace: `${appConfig.storagePrefix}-vben-v1`,
    overrides: appPreferences,
  });
  const { bootstrap } = await import("./bootstrap");
  await bootstrap();
}

void start().catch(() => {
  const en = preferences.app.locale === "en-US";
  showAppLoadError(
    en ? "Unable to load the app. Please try again." : "页面加载失败，请稍后重试。",
    en ? "Reload" : "重新加载",
  );
});
