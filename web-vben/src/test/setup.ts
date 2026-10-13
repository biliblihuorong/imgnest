import { config } from "@vue/test-utils";
import { i18n } from "@vben/locales";
import { beforeEach, onTestFinished } from "vitest";
import { disposePinia, getActivePinia } from "pinia";
import { setSessionToken } from "@/lib/session";

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
  setSessionToken(null);
  i18n.global.locale.value = "zh-CN";
  // 全部组件 afterEach/unmount 完成后，才撤销 store 的全局会话订阅。
  onTestFinished(() => {
    const pinia = getActivePinia();
    if (pinia) disposePinia(pinia);
  });
});

// jsdom 未实现滚动 API；上传页入队后会调用 scrollIntoView
if (!Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = () => {};
}
