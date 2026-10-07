import { defineConfig, type DefaultTheme } from "vitepress";

const REPO = "https://github.com/biliblihuorong/imgnest";

/**
 * 文档目录：新增一篇文档 = 在 docs/guide/（英文在 docs/en/guide/）放一个 .md，
 * 再往这里加一行。两种语言共用同一份结构，只是标题不同。
 */
const GUIDE: { title: [zh: string, en: string]; pages: [slug: string, zh: string, en: string][] }[] = [
  {
    title: ["开始", "Start"],
    pages: [
      ["introduction", "ImgNest 是什么", "What is ImgNest"],
      ["getting-started", "快速开始", "Getting started"],
      ["configuration", "配置", "Configuration"],
    ],
  },
  {
    title: ["使用", "Usage"],
    pages: [
      ["storage-and-policies", "存储与规则", "Storage and policies"],
      ["privacy-and-trash", "隐私与回收站", "Privacy and recycle bin"],
      ["lsky-api", "蓝空 API 与 PicGo", "Lsky API and PicGo"],
      ["frontends", "界面与主题", "Interface and themes"],
    ],
  },
];

function sidebar(lang: 0 | 1): DefaultTheme.SidebarItem[] {
  const base = lang === 0 ? "/guide/" : "/en/guide/";
  return GUIDE.map((group) => ({
    text: group.title[lang],
    items: group.pages.map((page) => ({ text: page[1 + lang], link: base + page[0] })),
  }));
}

export default defineConfig({
  srcDir: "docs",
  title: "ImgNest",
  cleanUrls: true,
  lastUpdated: false,
  appearance: true,
  // 文档里有指向本机服务的示例地址。
  ignoreDeadLinks: "localhostLinks",
  // 部署到子路径（例如 GitHub Pages 的 /imgnest/）时设置环境变量 SITE_BASE。
  base: process.env.SITE_BASE || "/",
  head: [["link", { rel: "icon", type: "image/svg+xml", href: `${process.env.SITE_BASE || "/"}logo.svg` }]],

  themeConfig: {
    logo: "/logo.svg",
    socialLinks: [{ icon: "github", link: REPO }],
    search: {
      provider: "local",
      options: {
        locales: {
          root: {
            translations: {
              button: { buttonText: "搜索文档", buttonAriaLabel: "搜索文档" },
              modal: {
                displayDetails: "显示详情",
                resetButtonTitle: "清空",
                backButtonTitle: "关闭",
                noResultsText: "没有找到相关内容：",
                footer: { selectText: "打开", navigateText: "切换", closeText: "关闭" },
              },
            },
          },
        },
      },
    },
  },

  locales: {
    root: {
      label: "简体中文",
      lang: "zh-CN",
      description: "自托管图床：Go 单二进制，多用户、多存储，原图与 WebP 各存一份，兼容蓝空图床 v1 API。",
      themeConfig: {
        nav: [
          { text: "首页", link: "/" },
          { text: "文档", link: "/guide/introduction", activeMatch: "^/guide/" },
        ],
        sidebar: { "/guide/": sidebar(0) },
        editLink: { pattern: `${REPO}/edit/main/website/docs/:path`, text: "在 GitHub 上编辑此页" },
        docFooter: { prev: "上一篇", next: "下一篇" },
        outline: { label: "本页内容", level: [2, 3] },
        returnToTopLabel: "回到顶部",
        sidebarMenuLabel: "目录",
        darkModeSwitchLabel: "深浅色",
        lightModeSwitchTitle: "切换到浅色",
        darkModeSwitchTitle: "切换到深色",
        langMenuLabel: "切换语言",
      },
    },
    en: {
      label: "English",
      lang: "en-US",
      description:
        "Self-hosted image hosting: one Go binary, multiple users and storages, original plus WebP, compatible with the Lsky Pro v1 API.",
      themeConfig: {
        nav: [
          { text: "Home", link: "/en/" },
          { text: "Docs", link: "/en/guide/introduction", activeMatch: "^/en/guide/" },
        ],
        sidebar: { "/en/guide/": sidebar(1) },
        editLink: { pattern: `${REPO}/edit/main/website/docs/:path`, text: "Edit this page on GitHub" },
        outline: { level: [2, 3] },
      },
    },
  },
});
