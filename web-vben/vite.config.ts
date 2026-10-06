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
    // Independent Vben output; the legacy web/dist is never overwritten.
    outDir: "dist",
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.ts"],
    testTimeout: 20000,
    setupFiles: ["./src/test/setup.ts"],
  },
});
