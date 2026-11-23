import tailwindcss from "@tailwindcss/vite";
import vue from "@vitejs/plugin-vue";
import { defineConfig } from "vitest/config";
import { appConfigHtmlPlugin } from "./build/app-config-html.ts";
import { viteTailwindReferencePlugin } from "./vendor/vben/build/tailwind-reference.ts";

export default defineConfig({
  plugins: [
    vue(),
    viteTailwindReferencePlugin(),
    tailwindcss(),
    // index.html 的站名、favicon、加载画面图标都来自 src/app.config.ts（admin-kit 约定）。
    appConfigHtmlPlugin(new URL("./src/app.config.ts", import.meta.url)),
  ],
  resolve: {
    alias: {
      "@": new URL("./src", import.meta.url).pathname,
    },
    dedupe: ["vue", "vue-router", "pinia"],
  },
  build: {
    outDir: "dist",
    // ClassicLayout（Vben 原生布局整套）压缩后约 700 kB，但它按需懒加载，
    // 只在用户切到经典布局时下载；默认的 Marvis 布局不受影响，所以放宽告警线。
    chunkSizeWarningLimit: 800,
  },
  server: {
    // docs.test.ts 读取 website/docs 核对帮助链接；只额外放开这一个目录。
    fs: { allow: [".", "../website/docs"] },
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.ts", "build/**/*.test.ts"],
    testTimeout: 20000,
    setupFiles: ["./src/test/setup.ts"],
  },
});
