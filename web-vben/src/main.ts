import { initPreferences, preferences } from "@vben/preferences";
import "@vben/styles";
import "@vben/styles/naive";
import { imgnestPreferences } from "./integrations/vben/preferences";
import "./assets/app.css";
import "./assets/marvis.css";

async function start(): Promise<void> {
  await initPreferences({ namespace: "imgnest-vben-v1", overrides: imgnestPreferences });
  const { bootstrap } = await import("./bootstrap");
  await bootstrap();
}

void start().catch(() => {
  const root = document.getElementById("app");
  if (root)
    root.textContent =
      preferences.app.locale === "en-US"
        ? "Unable to load the app. Please refresh."
        : "页面加载失败，请刷新后重试。";
});
