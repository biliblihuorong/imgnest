import tailwindcss from "@tailwindcss/vite";
import vue from "@vitejs/plugin-vue";
import { defineConfig } from "vitest/config";
import { viteTailwindReferencePlugin } from "./vendor/vben/build/tailwind-reference.ts";

export default defineConfig({
  plugins: [vue(), viteTailwindReferencePlugin(), tailwindcss()],
  resolve: {
    alias: {
      "@": new URL("./src", import.meta.url).pathname,
    },
    dedupe: ["vue", "vue-router", "pinia"],
  },
  build: {
    outDir: "dist",
  },
  server: {
    // docs.test.ts 读取 website/docs 核对帮助链接；只额外放开这一个目录。
    fs: { allow: [".", "../website/docs"] },
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.ts"],
    testTimeout: 20000,
    setupFiles: ["./src/test/setup.ts"],
  },
});
