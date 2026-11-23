import { updatePreferences } from "@vben/preferences";
import { reactive } from "vue";
import { appConfig } from "@/app.config";
import { loadSite as loadSiteHook } from "@/app.hooks";

/** 外壳显示用的品牌信息。静态默认值来自 app.config.ts，运行时可由 app.hooks.ts 的 loadSite 覆盖。 */
export interface SiteBrand {
  /** 侧栏、登录页、经典布局顶栏显示的站名。 */
  name: string;
  /** 浏览器标题后缀（「页面 - 标题」）。 */
  title: string;
  logo: string;
}

export const site = reactive<SiteBrand>({
  name: appConfig.name,
  title: appConfig.name,
  logo: appConfig.logo,
});

/** 更新品牌；没给的字段回到 app.config.ts 的默认值。logo 同时用作浏览器图标。 */
export function applySite(info: Partial<SiteBrand>): void {
  site.name = info.name || appConfig.name;
  site.title = info.title || site.name;
  site.logo = info.logo || appConfig.logo;
  updatePreferences({
    app: { name: site.name },
    logo: { source: site.logo, sourceDark: site.logo },
  });
  setFavicon(site.logo === appConfig.logo ? "" : site.logo);
}

/** 空字符串表示恢复 index.html 里构建时注入的图标。 */
function setFavicon(url: string): void {
  const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
  if (!link) return;
  if (link.dataset.original === undefined) link.dataset.original = link.getAttribute("href") ?? "";
  link.href = url || link.dataset.original;
}

const LOAD_TIMEOUT_MS = 3000;

/** 启动时调用：问 app.hooks.ts 要运行时品牌。永不抛错，最多等 3 秒。 */
export async function loadSite(): Promise<void> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    const info = await Promise.race([
      loadSiteHook(),
      new Promise<never>((_, reject) => {
        timer = setTimeout(() => reject(new Error("timeout")), LOAD_TIMEOUT_MS);
      }),
    ]);
    if (info) applySite(info);
  } catch {
    // 保持 app.config.ts 的默认值。
  } finally {
    clearTimeout(timer);
  }
}
