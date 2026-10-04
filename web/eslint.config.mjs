import skipFormatting from "@vue/eslint-config-prettier/skip-formatting";
import { defineConfigWithVueTs, vueTsConfigs } from "@vue/eslint-config-typescript";
import pluginVue from "eslint-plugin-vue";

export default defineConfigWithVueTs(
  {
    name: "imgnest/ignores",
    ignores: ["dist/**", "coverage/**", "node_modules/**", "src/api/schema.d.ts"],
  },
  {
    name: "imgnest/files",
    files: ["**/*.{ts,mts,tsx,vue}"],
  },
  pluginVue.configs["flat/recommended"],
  vueTsConfigs.recommended,
  skipFormatting,
  {
    name: "imgnest/rules",
    rules: {
      "vue/multi-word-component-names": ["error", { ignores: ["App"] }],
    },
  },
);
