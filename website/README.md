# ImgNest 项目网站

首页 + 文档，基于 [VitePress](https://vitepress.dev) 1.6.4。这是一个独立的工作区，和 `web-vben/` 不共用依赖。

## 运行

```bash
cd website
pnpm install --frozen-lockfile
pnpm dev        # 本地预览，改动即时生效
pnpm build      # 输出静态文件到 .vitepress/dist
pnpm preview    # 预览构建结果
```

需要 Node 22.12 或更高版本。这里故意不放 `.nvmrc`：静态托管平台会按它切换到精确版本，而平台预装的版本往往对不上，所以 Node 版本在平台的项目设置里选。

部署到子路径（例如 GitHub Pages 的 `/imgnest/`）时，构建前设置 `SITE_BASE=/imgnest/`。

## 改什么去哪里

| 想改的内容               | 文件                                                    |
| ------------------------ | ------------------------------------------------------- |
| 文档正文                 | `docs/guide/*.md`（中文）、`docs/en/guide/*.md`（英文） |
| 文档目录、顶栏、搜索文案 | `.vitepress/config.ts`                                  |
| 首页文案                 | `.vitepress/theme/home/copy.ts`                         |
| 配色、圆角、阴影         | `.vitepress/theme/marvis.css` 第 1 节                   |
| 首页结构与滚动演示       | `.vitepress/theme/home/*.vue`                           |
| Logo                     | `docs/public/logo.svg`                                  |

### 新增一篇文档

1. 在 `docs/guide/` 放一个 `.md` 文件，英文版放到 `docs/en/guide/` 的同名文件里。
2. 在 `.vitepress/config.ts` 的 `GUIDE` 里加一行：`["文件名", "中文标题", "English title"]`。

文档里可以用 VitePress 的 Markdown 扩展：`::: tip`、`::: warning` 提示块，`::: code-group` 多标签代码块，表格和代码高亮。正文里出现花括号变量（如路径模板）时，用 `<div v-pre>` 包起来，避免被当成模板语法。

### 首页的滚动演示

- 「界面」一段：`copy.ts` 里每个 `tour.steps[].state` 决定滚到这一步时演示切到哪种布局、深浅色和语言。演示本身是 `AdminMock.vue` 用 HTML/CSS 画的，不是截图，界面变了改这个文件。
- 「润物细无声」一段：三个面板在 `EnginePanel.vue`，数字来自 `deploy/config.example.yaml` 的默认值，改默认值时记得同步 `copy.ts` 里的 `engine.limits.rows`。

## 风格

沿用新版管理端的 Marvis 风格：灰底、白色漂浮卡片、大圆角、柔和阴影。设计令牌的取值与 `web-vben/src/assets/marvis.css` 保持一致。

## 文档布局回归

构建后启动 `pnpm preview --host 127.0.0.1 --port 4174`，再用已有的 Playwright 浏览器工具运行：

```bash
node e2e/check-layout.mjs --origin http://127.0.0.1:4174
```

脚本检查中英文、深浅色、1280/1440/1920 宽屏，以及手机、平板和较矮屏幕：目录卡片按内容收紧、文字保留标记线内缩、长目录可独立滚动、锚点跳转后目录仍在视口内。截图和测量数据默认保存到 `/tmp/imgnest-doc-layout`；用 `--output` 指定其他位置，用 `--chromium` 指定已有 Chromium。

浏览器工具不加入网站依赖；若 Playwright 在外部工具目录，可通过 `IMGNEST_E2E_PLAYWRIGHT_MODULE` 指向其 `index.mjs`。布局验收还需打开生成的截图检查整体效果，不能只凭顶部坐标判定通过。
