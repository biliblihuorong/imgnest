import { config } from "@vue/test-utils";
import { i18n } from "@vben/locales";
import { beforeEach } from "vitest";

const vendorDictionaries = import.meta.glob(
  "../../vendor/vben/packages/locales/src/langs/*/*.json",
  { eager: true, import: "default" },
);
for (const [path, messages] of Object.entries(vendorDictionaries)) {
  const match = path.match(/langs\/([^/]+)\/([^/]+)\.json$/);
  if (match) i18n.global.mergeLocaleMessage(match[1]!, { [match[2]!]: messages });
}

const dictionaries = import.meta.glob("../locales/messages/*/*.json", {
  eager: true,
  import: "default",
});
for (const [path, messages] of Object.entries(dictionaries)) {
  const match = path.match(/messages\/([^/]+)\/([^/]+)\.json$/);
  if (match) i18n.global.mergeLocaleMessage(match[1]!, { [match[2]!]: messages });
}
i18n.global.locale.value = "zh-CN";
config.global.plugins.push(i18n);
beforeEach(() => {
  i18n.global.locale.value = "zh-CN";
});
