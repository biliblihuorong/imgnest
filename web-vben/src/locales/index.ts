import type { App } from "vue";
import { loadLocalesMapFromDir, setupI18n } from "@vben/locales";
import { preferences } from "@vben/preferences";

const messages = loadLocalesMapFromDir(
  /\.\/messages\/([^/]+)\/(.*)\.json$/,
  import.meta.glob("./messages/*/*.json"),
);

export async function setupAppI18n(app: App): Promise<void> {
  const locale = preferences.app.locale === "en-US" ? "en-US" : "zh-CN";
  await setupI18n(app, {
    defaultLocale: locale,
    missingWarn: import.meta.env.DEV,
    loadMessages: async (language) => (await messages[language]?.())?.default ?? {},
  });
}
