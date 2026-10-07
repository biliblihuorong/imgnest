import type { Theme } from "vitepress";
import DefaultTheme from "vitepress/theme";
import HomePage from "./home/HomePage.vue";
import "./marvis.css";

/** 默认主题 + Marvis 风格的样式覆盖；首页是自定义组件，文档页全部来自 Markdown。 */
export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component("HomePage", HomePage);
  },
} satisfies Theme;
