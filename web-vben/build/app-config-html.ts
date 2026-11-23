import { readFileSync } from "node:fs";
import type { Plugin } from "vite";

/**
 * 把 src/app.config.ts 里的品牌信息注入 index.html，让 app.config.ts 成为唯一的配置源：
 *
 * - %APP_NAME%            ← appConfig.name（<title> 的初始值）
 * - %APP_LOGO%            ← `import logo from "..."` 的文件（favicon 与启动加载画面的图标）
 * - %APP_STORAGE_PREFIX%  ← appConfig.storagePrefix（加载画面在脚本加载前读取深浅色/强调色偏好）
 *
 * index.html 在任何脚本执行前就要用到这些值，没法 import TS，所以这里按文本读取。
 * 读不到时直接让构建失败，不会悄悄用错的值。
 */
export interface AppHtmlBrand {
  name: string;
  /** 以 / 开头、相对项目根目录的路径，交给 Vite 处理成带哈希的资源。 */
  logo: string;
  storagePrefix: string;
}

export function readAppBrand(source: string): AppHtmlBrand {
  // 只在 appConfig 对象里找，避免匹配到前面的类型声明或后面菜单项的 name。
  const start = source.search(/export\s+const\s+appConfig\b/);
  const config = start >= 0 ? source.slice(start) : "";
  const name = /^\s*name:\s*["'`]([^"'`\n]+)["'`]/m.exec(config)?.[1];
  const storagePrefix = /^\s*storagePrefix:\s*["'`]([\w.-]+)["'`]/m.exec(config)?.[1];
  const logoImport = /^import\s+logo\s+from\s+["']([^"']+)["']/m.exec(source)?.[1];
  const missing = [
    !name && "appConfig.name",
    !storagePrefix && "appConfig.storagePrefix",
    !logoImport && 'import logo from "…"',
  ].filter(Boolean);
  if (missing.length > 0)
    throw new Error(`app-config-html: 在 src/app.config.ts 里找不到 ${missing.join("、")}`);
  return {
    name: name!,
    storagePrefix: storagePrefix!,
    logo: toRootPath(logoImport!),
  };
}

function toRootPath(specifier: string): string {
  if (specifier.startsWith("@/")) return `/src/${specifier.slice(2)}`;
  if (specifier.startsWith("./")) return `/src/${specifier.slice(2)}`;
  if (specifier.startsWith("/")) return specifier;
  throw new Error(
    `app-config-html: logo 请用 "@/…" 或 "./…" 引入项目内的文件，当前是 "${specifier}"`,
  );
}

const escapeHtml = (value: string) =>
  value.replace(
    /[&<>"']/g,
    (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[char]!,
  );

export function appConfigHtmlPlugin(configFile: URL): Plugin {
  return {
    name: "admin-kit:app-config-html",
    transformIndexHtml: {
      // 必须在 Vite 处理 <img>/<link> 资源路径之前替换，logo 才会被打包成带哈希的文件。
      order: "pre",
      handler(html) {
        const brand = readAppBrand(readFileSync(configFile, "utf8"));
        return html
          .replaceAll("%APP_NAME%", escapeHtml(brand.name))
          .replaceAll("%APP_LOGO%", brand.logo)
          .replaceAll("%APP_STORAGE_PREFIX%", brand.storagePrefix);
      },
    },
  };
}
