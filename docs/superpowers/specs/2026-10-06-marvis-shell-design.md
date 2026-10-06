# Marvis 外壳与外观设置 设计（2026-10-06）

参考：[MarvisUITemplate](https://cnb.cool/mint/MarvisUITemplate)（MIT，纯静态 HTML/CSS，只移植设计语言，不复制代码）。效果图：<https://claude.ai/artifact/HD8sTQD1tXmhLCX1EJyn3Q>（临时示意，数据与部分交互是编的，以本文为准）。

## 目标

在 `web-vben/` 内新增一套 Marvis 风格外壳，作为该前端的默认外观；现有 Vben 布局保留为「经典布局」，用户可随时切回。两套外壳共用同一份路由、页面、API 层、搜索、右键菜单和灯箱逻辑。

成功标准：

1. 新浏览器打开 `web-vben` 构建，看到的是 Marvis 外壳；在设置里切到经典布局后与今天的界面一致，切换不刷新页面、不丢登录态。
2. 「我的图片」的统一搜索、多选批量、右键菜单、灯箱在两套外壳下行为完全一致。
3. 现有前端测试全部通过；新增代码有组件测试。

## 范围

**本期（纯前端，不改后端、不改 OpenAPI、不加依赖）：**

- 外壳切换机制，默认 Marvis。
- Marvis 外壳：漂浮侧栏、`Ctrl K` 全局检索、设置弹窗、窄屏抽屉。
- 「我的图片」与相册详情：Marvis 下详情改为右侧预览面板。
- 侧栏相册分组：展开列出相册，带「新建相册」入口。
- 外观设置：布局、深色模式、强调色、圆角、紧凑侧栏。
- Marvis 设计令牌与 Naive UI 主题贴合。

**不做：**

- 管理员站点默认主题（记为后续，见文末）。
- 任意自定义 CSS 注入、主题包上传。
- 重写管理后台表格/表单页的内部结构（靠令牌贴近风格）。
- 改动旧版 `web/`；改动发布默认（默认二进制仍嵌 `web/`）。
- Marvis 模板里的 AI 助手首页、对话页、推荐卡片。

## 架构

```
App.vue
└─ AppLayout.vue            分发器：按 shell 偏好渲染下面之一（均为异步组件）
   ├─ ClassicLayout.vue     现 AppLayout 的内容原样搬入（Vben BasicLayout）
   └─ MarvisLayout.vue      新外壳
      ├─ MarvisSidebar.vue      品牌、搜索入口、导航、相册分组、用户卡片
      ├─ MarvisCommandPalette.vue   Ctrl K 全局检索
      └─ MarvisSettingsModal.vue    设置弹窗（外观 / 账号 / Token）
```

- 两套布局都用 `defineAsyncComponent` 加载，Marvis 用户不下载 `@vben/layouts` 的布局代码。
- 导航数据继续来自 `integrations/vben/navigation.ts` 的 `workspaceMenus()`，两套外壳读同一份菜单定义，新增页面只改一处。
- `integrations/vben/session.ts` 不变。它投影到 Vben store 的菜单和标签页在 Marvis 下只是不被渲染。
- 登录/注册页（`meta.bare`）不经过 `AppLayout`，本期只跟随令牌变化，不改结构。

### 外壳偏好

新增 `src/integrations/shell/useShell.ts`：

- 类型 `Shell = "marvis" | "classic"`，默认 `"marvis"`。
- 存 `localStorage["imgnest-shell"]`；读写都包 try/catch，存储不可用时用默认值且不报错。读到未知值按默认处理。
- 模块级单例 `shallowRef`，导出 `shell` 与 `setShell()`；`setShell` 同步写 `document.documentElement.dataset.shell`，启动时在 `bootstrap()` 里应用一次，避免首屏闪烁。
- 这是视图偏好，不放进 Pinia（项目约定全局状态只放用户信息与站点配置）。

### 其余外观项复用 Vben 偏好

`@vben/preferences` 已持久化这些值，且 `App.vue` 里的 `useNaiveDesignTokens()` 会把它们同步给 Naive UI，所以不另存一份：

| 设置项 | 存储位置 | 说明 |
| --- | --- | --- |
| 深色模式 | `theme.mode`（light / dark / auto） | 现有行为 |
| 强调色 | `theme.colorPrimary` | 提供 5 个预设色 |
| 圆角 | `theme.radius` | 小 0.25 / 中 0.5 / 大 0.75 |
| 紧凑侧栏 | `sidebar.collapsed` | 两套外壳共用同一个开关 |

两套外壳共用同一个强调色和圆角，切换布局不会丢这些选择。默认值保持 Vben 现值不变，所以经典布局的默认观感与今天完全相同；Marvis 的 `#3b5bff` 作为预设色之一。

### 设计令牌

新增 `src/assets/marvis.css`，所有规则限定在 `html[data-shell="marvis"]` 下，经典布局不受影响：

- 覆盖 Vben 的语义变量（`--background`、`--card`、`--border`、`--muted` 等）为 Marvis 的中性灰阶，明暗各一组，暗色挂在 Vben 的 `.dark` 类下。Naive UI 通过现有的 token 同步自动跟随。
- Marvis 专有变量（`--mv-shadow-sm/md/lg`、`--mv-ease`、侧栏阴影）加 `--mv-` 前缀，避免和 Vben 变量冲突。
- 强调色与圆角不在此文件写死，取 Vben 的 `--primary` / `--radius`。
- 少量 Naive UI 贴合规则（下拉菜单、弹窗、分页的圆角与阴影）也写在这里，限定同一选择器。

## 组件设计

### MarvisLayout

- 桌面：灰底，左侧 244px 漂浮白卡侧栏，右侧 `<RouterView>` 占满。紧凑侧栏时侧栏收为图标列。
- ≤768px：侧栏变抽屉（左上角菜单按钮 + 遮罩），路由切换后自动收起。
- 匿名访问 `/gallery` 时侧栏只显示画廊和登录入口，与现有 `workspaceMenus(undefined, …)` 的结果一致。
- 退出登录沿用现 `AppLayout` 的 `logout()`（先跳 `/login` 再等请求），提到 `components/layout/useLogout.ts` 供两套布局共用。

### MarvisSidebar

- 顶部：站点名（`site.siteName`）+ Logo；搜索入口按钮（显示 `Ctrl K` / `⌘K`）。
- 导航：`workspaceMenus()` 的顶层项；管理员的 `/admin` 子项渲染成带「管理」标题的分组。
- **相册分组**：「相册」项可展开，列出相册（调用现有 `listAlbums`，取第一页），点击进入 `/albums/:id`；分组末尾固定「新建相册」和「全部相册」两项。「新建相册」打开现有的 `AlbumFormModal`，保存成功后刷新列表并跳到新相册。相册的编辑、删除、设封面仍在 `/albums` 页完成。
- 底部用户卡片：头像（`useUserAvatar`）、显示名、设置按钮。未登录时显示登录按钮。

### MarvisCommandPalette

- `Ctrl/⌘ + K` 或点侧栏搜索入口打开；`Esc` 关闭；焦点进入输入框，关闭后还给触发元素。
- 输入内容按回车后跳转 `/images`，用与 `useImageQueryState` 相同的路由查询参数携带原始查询串，由统一搜索解析与校验。弹窗自身不重新实现搜索语法。
- 输入框下方显示现有 `useSearchSuggestions` 的建议和几条页面跳转项（上传、相册、回收站等，来自同一份菜单）。
- 未登录时不注册快捷键。

### MarvisSettingsModal

- 左侧分类 + 右侧内容，≤768px 时全屏、分类变横向滚动条。
- **外观**：布局（新版 / 经典）、深色模式、强调色、圆角、紧凑侧栏。改动即时生效。
- **账号**：嵌入现有 `AccountSettingsView` 的资料表单和 `ChangePasswordCard`。
- **API Token**：链接到 `/tokens` 页（Token 明文展示流程不搬进弹窗）。
- 经典布局下的入口：在 `ClassicLayout` 的用户下拉菜单里加一项「切换到新版布局」，直接调用 `setShell("marvis")`。

### 图片库

`ImageLibrary.vue` 现在同时负责工具栏、网格、右键菜单和详情抽屉。改动：

- 把详情内容从 `ImageDetailDrawer.vue` 抽成 `ImageDetailContent.vue`（元数据、EXIF、链接），抽屉只保留外壳。
- 新增 `ImageDetailPanel.vue`：右侧常驻面板，内含 `ImageDetailContent`；未选中时显示空状态。
- `ImageLibrary` 根据 `shell` 与视口宽度二选一：Marvis 且宽度 ≥1040px 用面板，其余情况（经典布局、窄屏）用现有抽屉。`lib.openDrawer(image)` 的调用点不变，只是呈现方式不同。
- Marvis 下单击卡片即选中并在面板显示详情；经典布局保持点文件名打开抽屉。
- **右键菜单不改逻辑**：继续用现有的 `menuOptions` / `onMenuSelect`，即「原图 / WebP / 缩略图 → URL / Markdown / HTML / BBCode」二级菜单、属性、公开切换、删除确认。只通过令牌调整外观。
- **灯箱不改逻辑**：继续用 `NImageGroup` + `NImage`。
- 统一搜索、多选批量栏、分页不改逻辑。

### 页面容器

现有页面用 `@vben/common-ui` 的 `Page` 组件，它依赖 Vben 布局提供的高度变量。实施第一步先验证它在 `MarvisLayout` 下的表现；若高度计算异常，由 `MarvisLayout` 提供同名 CSS 变量，不改各页面。

## 错误处理

- 侧栏相册列表加载失败：分组内显示一行重试，不影响其余导航；不弹全局错误。
- `localStorage` 不可用：使用默认外壳，设置仍可在当前会话内切换。
- 异步布局组件加载失败：回退渲染另一套布局并提示一次，避免白屏。
- 其余请求错误沿用各页面现有处理（`formatApiError`）。

## 可访问性

- 侧栏、检索弹窗、设置弹窗全程可用键盘操作，焦点环沿用 `app.css` 的 `:focus-visible`。
- 弹窗有 `role="dialog"` 与可读标题，打开时锁定焦点。
- 尊重 `prefers-reduced-motion`（`app.css` 已有全局规则）。
- 新增文案全部走 i18n，中英文各一份，放 `locales/messages/*/shell.json`。

## 测试

- `useShell`：默认值、持久化、未知值回退、存储抛错时不崩。
- `AppLayout` 分发：按偏好渲染对应布局；切换后另一套卸载。
- `MarvisSidebar`：按角色和 `galleryEnabled` 渲染菜单；相册展开、加载失败重试、新建相册后刷新。
- `MarvisCommandPalette`：快捷键开关、回车跳转并带上查询串、未登录不响应。
- `MarvisSettingsModal`：各外观项调用对应的偏好更新；切到经典布局。
- `ImageLibrary`：Marvis 宽屏用面板、窄屏与经典用抽屉；右键菜单选项与复制结果的现有测试保持通过。
- 全量：`make fe-*` 对应的 vben 测试、类型检查、lint；`scripts/check-frontend-selection.sh` 保持通过。
- 浏览器验收：起开发服务器，在两套外壳下各走一遍上传、搜索、右键复制、灯箱、相册新建、深浅色切换，桌面与手机宽度各一次。

## 文档同步

- `docs/planning/dual-frontend-build.md`：补充 `web-vben` 内含两套外壳及默认值。
- `README.md`「可选的 Vben Naive 前端」一节：一句话说明。
- `docs/spec.md` 若有前端外观相关描述则同步；无则不动。

## 风险

- **Vben 页面容器依赖布局变量**：见「页面容器」，先验证再定。
- **令牌覆盖不到的 Naive UI 细节**：管理后台表格/表单页只能贴近。验收时逐页看，个别页面需要重写的另行排期。
- **vendor 升级**：只依赖 Vben 的公开 CSS 变量名和 `@vben/preferences` 的公开 API，不改 `vendor/` 下任何文件。

## 后续（不在本期）

管理员站点默认主题：`/api/site` 增加默认外壳与强调色字段，后台设置页可配，用户本地选择优先于站点默认。需要新增版本化迁移并更新 `docs/openapi.yaml`、`docs/spec.md`，另开设计。
