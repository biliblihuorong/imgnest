# Marvis 外壳与外观设置 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 `web-vben/` 内新增默认启用的 Marvis 风格外壳，保留现有 Vben 布局为可切回的「经典布局」，并提供外观设置。

**Architecture:** `AppLayout.vue` 变为分发器，按 `useShell` 的偏好异步加载 `ClassicLayout`（现有内容原样搬入）或新的 `MarvisLayout`。路由、页面、API 层、搜索、右键菜单、灯箱全部共用；外观项除「布局」外复用 `@vben/preferences`。Marvis 令牌限定在 `html[data-shell="marvis"]` 下。

**Tech Stack:** Vue 3.5 `<script setup lang="ts">`、Naive UI 2.45、`@vben/*`（vendor 内，不改）、Pinia、vue-router、Vitest + @vue/test-utils、Tailwind 4。

**Spec:** [docs/superpowers/specs/2026-10-06-marvis-shell-design.md](../specs/2026-10-06-marvis-shell-design.md)

## Global Constraints

- 只改 `web-vben/src/` 与 `docs/`、`README.md`。不改 `web/`、`web-vben/vendor/`、后端、`docs/openapi.yaml`、发布默认。
- 不新增依赖，不升级版本（`docs/versions.md`）。图标只用 `@vben/icons` 已导出的。
- 不改现有交互逻辑：统一搜索、多选批量、分页、右键菜单选项与复制结果、灯箱、上传、相册表单、回收站、Token、验证码、退出登录顺序、路由守卫。现有测试的断言不改；只允许因文件搬迁而改 import 路径。
- 请求只写在 `src/api/`；本计划不新增 API 函数。
- 新增文案全部走 i18n：`src/locales/messages/zh-CN/shell.json` 与 `en-US/shell.json`，键前缀 `shell.`（目录 glob 自动加载）。
- `localStorage` 读写一律 try/catch。视图偏好不进 Pinia。
- 密钥、Token 不进日志；Token 明文流程不搬进设置弹窗。
- 提交信息用 Conventional Commits，结尾带 `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`。
- 测试在容器内跑（本机 Node 22 不满足 `>=24.21`）。下文 `FE <cmd>` 表示：
  `docker compose -f deploy/compose.dev.yaml run --rm dev sh -c 'cd web-vben && <cmd>'`

## Review Focus

1. `localStorage["imgnest-shell"]` 是未知值、空串或读取抛错 → 用 `marvis`，不报错（Task 1）。
2. 用户自选过强调色或圆角后切换布局 → 自选值保持；只自选了其中一项时另一项仍跟随布局（Task 1）。
3. 登出后以另一个账号登录 → 侧栏不残留上一个账号的相册列表（Task 4）。
4. 详情打开时视口跨过 1040px → 面板与抽屉互换，仍显示同一张图，EXIF 不重复请求出错（Task 7）。
5. 输入法组合输入中、或焦点在别的弹窗里按 `Ctrl K` / 回车 → 不误触发跳转；匿名访问不响应快捷键（Task 5）。
6. 布局异步 chunk 加载失败 → 回退到另一套布局，不白屏（Task 2）。

---

### Task 1: 外壳偏好 `useShell`

**Files:**
- Create: `web-vben/src/integrations/shell/useShell.ts`
- Test: `web-vben/src/integrations/shell/useShell.test.ts`
- Modify: `web-vben/src/bootstrap.ts`（`createApp` 之前调用 `initShell()`）

**Interfaces:**
- Produces:
  ```ts
  export type Shell = "marvis" | "classic";
  export const SHELL_STORAGE_KEY = "imgnest-shell";
  export const SHELL_DEFAULTS: Record<Shell, { colorPrimary: string; radius: string }> = {
    classic: { colorPrimary: "hsl(212 100% 45%)", radius: "0.5" },
    marvis: { colorPrimary: "hsl(230 100% 62%)", radius: "0.75" }, // ≈ #3b5bff
  };
  export const shell: Readonly<ShallowRef<Shell>>;
  /** 读存储 → 写 <html data-shell> → 按「跟随布局」规则对齐强调色与圆角。可重复调用。 */
  export function initShell(): void;
  export function setShell(next: Shell): void;
  /** 把强调色与圆角写回当前外壳默认值。 */
  export function resetAppearance(): void;
  ```
- Consumes: `preferences`、`updatePreferences` from `@vben/preferences`（`theme.colorPrimary`、`theme.radius`）。

- [ ] **Step 1: 写失败测试**（每个用例前 `localStorage.clear()`、`updatePreferences({ theme: SHELL_DEFAULTS.classic })`、删除 `data-shell`）

  ```ts
  it("defaults to marvis and marks <html>", () => { initShell(); expect(shell.value).toBe("marvis"); expect(document.documentElement.dataset.shell).toBe("marvis"); });
  it("adopts marvis defaults when the user never chose a colour", () => { initShell(); expect(preferences.theme.colorPrimary).toBe(SHELL_DEFAULTS.marvis.colorPrimary); expect(preferences.theme.radius).toBe("0.75"); });
  it("restores a stored classic choice", () => { localStorage.setItem(SHELL_STORAGE_KEY, "classic"); initShell(); expect(shell.value).toBe("classic"); expect(preferences.theme.colorPrimary).toBe(SHELL_DEFAULTS.classic.colorPrimary); });
  it.each(["", "vben", "null"])("falls back to marvis for stored %j", (v) => { localStorage.setItem(SHELL_STORAGE_KEY, v); initShell(); expect(shell.value).toBe("marvis"); });
  it("survives a throwing storage", () => { vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => { throw new Error("blocked"); }); vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("blocked"); }); initShell(); expect(() => setShell("classic")).not.toThrow(); expect(shell.value).toBe("classic"); });
  it("setShell persists and swaps untouched defaults", () => { initShell(); setShell("classic"); expect(localStorage.getItem(SHELL_STORAGE_KEY)).toBe("classic"); expect(preferences.theme.colorPrimary).toBe(SHELL_DEFAULTS.classic.colorPrimary); expect(preferences.theme.radius).toBe("0.5"); });
  it("keeps a colour the user picked, still follows radius", () => { initShell(); updatePreferences({ theme: { colorPrimary: "hsl(160 84% 34%)" } }); setShell("classic"); expect(preferences.theme.colorPrimary).toBe("hsl(160 84% 34%)"); expect(preferences.theme.radius).toBe("0.5"); });
  it("resetAppearance returns to the current shell defaults", () => { initShell(); updatePreferences({ theme: { colorPrimary: "hsl(160 84% 34%)", radius: "0.25" } }); resetAppearance(); expect(preferences.theme).toMatchObject(SHELL_DEFAULTS.marvis); });
  ```

- [ ] **Step 2: 跑测试确认失败** — `FE pnpm vitest run src/integrations/shell` → FAIL（模块不存在）
- [ ] **Step 3: 实现 `useShell.ts`**。模块级 `shallowRef`；「未自选」的判定是当前值等于**另一套**外壳的默认值（`initShell` 时）或**上一套**外壳的默认值（`setShell` 时），两项独立判断。
- [ ] **Step 4: 在 `bootstrap()` 第一行调用 `initShell()`**
- [ ] **Step 5: 跑测试确认通过** — 同 Step 2 命令 → PASS；再跑 `FE pnpm vitest run` 确认全量仍通过
- [ ] **Step 6: Commit** — `feat(web-vben): add shell preference with per-shell appearance defaults`

---

### Task 2: 布局分发器与经典布局

**Files:**
- Create: `web-vben/src/components/layout/ClassicLayout.vue`（现 `AppLayout.vue` 内容原样搬入）
- Create: `web-vben/src/components/layout/useLogout.ts`
- Create: `web-vben/src/components/layout/MarvisLayout.vue`（本任务只放最小骨架：`<div data-testid="marvis-layout"><RouterView /></div>`）
- Modify: `web-vben/src/components/layout/AppLayout.vue`（改为分发器）
- Modify: `web-vben/src/components/layout/locale-lifecycle.test.ts`、`logout-lifecycle.test.ts`（仅把 import 的 `./AppLayout.vue` 改为 `./ClassicLayout.vue`）
- Create: `web-vben/src/locales/messages/{zh-CN,en-US}/shell.json`
- Test: `web-vben/src/components/layout/AppLayout.test.ts`、`useLogout.test.ts`

**Interfaces:**
- Consumes: `shell`、`setShell` from Task 1。
- Produces:
  ```ts
  // useLogout.ts —— 行为与现 AppLayout.logout() 完全一致：防重入；先 router.replace("/login") 再 await 请求
  export function useLogout(): { loggingOut: Readonly<ShallowRef<boolean>>; logout: () => Promise<void> };
  ```
  - `AppLayout.vue`：无 props；用 `defineAsyncComponent` 加载两套布局，`shell.value` 决定渲染哪一套。
  - `ClassicLayout.vue`：`accountMenus` 首项新增 `{ text: t("shell.switchToMarvis"), handler: () => setShell("marvis") }`。
  - i18n 键（本任务）：`shell.switchToMarvis` =「切换到新版布局」/ "Switch to the new layout"；`shell.layoutLoadFailed` =「新布局加载失败，已切回另一套」/ "Layout failed to load; switched to the other one"。

- [ ] **Step 1: 写失败测试**

  ```ts
  // AppLayout.test.ts（stub 两套布局为带 data-testid 的空组件）
  it("renders the marvis layout by default", async () => { /* initShell(); mount; flushPromises */ expect(w.find('[data-testid="marvis-layout"]').exists()).toBe(true); expect(w.find('[data-testid="classic-layout"]').exists()).toBe(false); });
  it("swaps layouts without remounting the app when the shell changes", async () => { setShell("classic"); await flushPromises(); expect(w.find('[data-testid="classic-layout"]').exists()).toBe(true); expect(w.find('[data-testid="marvis-layout"]').exists()).toBe(false); });
  it("falls back to the other layout when a chunk fails to load", async () => { /* vi.mock MarvisLayout 的动态 import 为 reject */ expect(w.find('[data-testid="classic-layout"]').exists()).toBe(true); expect(shell.value).toBe("classic"); });
  // useLogout.test.ts
  it("navigates to /login before the logout request settles", async () => { /* auth.logout 返回挂起的 promise */ void logout(); await flushPromises(); expect(router.currentRoute.value.path).toBe("/login"); expect(loggingOut.value).toBe(true); });
  it("ignores a second call while logging out", async () => { void logout(); void logout(); expect(auth.logout).toHaveBeenCalledTimes(1); });
  ```

- [ ] **Step 2: 跑测试确认失败** — `FE pnpm vitest run src/components/layout` → 新测试 FAIL
- [ ] **Step 3: 搬迁与实现**。`ClassicLayout` 根节点不加包装元素（保持 Vben 布局的高度行为）；`defineAsyncComponent` 用 `onError` 回调调用 `setShell(另一套)` 并 `message.warning(t("shell.layoutLoadFailed"))`，只回退一次，避免两套都失败时循环。
- [ ] **Step 4: 跑测试确认通过** — 同 Step 2 → PASS；两个 lifecycle 测试断言未改且通过
- [ ] **Step 5: Commit** — `refactor(web-vben): split AppLayout into shell dispatcher and ClassicLayout`

---

### Task 3: Marvis 令牌、布局与侧栏导航

**Files:**
- Create: `web-vben/src/assets/marvis.css`（在 `main.ts` 中紧随 `app.css` 引入）
- Create: `web-vben/src/components/layout/marvis/MarvisSidebar.vue`
- Create: `web-vben/src/components/layout/marvis/useNarrowViewport.ts`
- Modify: `web-vben/src/components/layout/MarvisLayout.vue`
- Modify: `shell.json`（两种语言）
- Test: `web-vben/src/components/layout/marvis/MarvisSidebar.test.ts`、`web-vben/src/components/layout/MarvisLayout.test.ts`

**Interfaces:**
- Consumes: `workspaceMenus(role, galleryEnabled)`、`useAuthStore`、`useSiteStore`、`useUserAvatar`、`useLogout`、`preferences.sidebar.collapsed`。
- Produces:
  ```ts
  // useNarrowViewport.ts
  export function useViewportBelow(px: number): Readonly<Ref<boolean>>; // 基于 @vueuse/core useMediaQuery
  // MarvisSidebar.vue
  defineProps<{ collapsed: boolean }>();
  defineEmits<{ navigate: []; "open-search": []; "open-settings": [] }>();
  // 具名插槽 #albums：渲染在「相册」导航项下方（Task 4 填充）
  ```
  - `MarvisLayout.vue`：提供 `provide("marvis-open-settings", () => void)`；≤768px 时侧栏为抽屉，`router.afterEach` 关闭抽屉。
  - `marvis.css` 变量：`--mv-shadow-sm`、`--mv-shadow-md`、`--mv-shadow-lg`、`--mv-shadow-side`、`--mv-ease: cubic-bezier(.22,1,.36,1)`；并在 `html[data-shell="marvis"]` 与 `html[data-shell="marvis"].dark` 下覆盖 Vben 的 `--background`、`--card`、`--popover`、`--muted`、`--border`、`--sidebar` 等语义变量为 Marvis 灰阶（亮：底 `#f7f7f8`、卡 `#ffffff`、线 `rgba(0,0,0,.06)`、主字 `#1c1c1e`、次字 `#71717a`；暗：底 `#111113`、卡 `#1c1c20`）。写入前先读 `web-vben/vendor/vben/packages/@core/base/design/src/design-tokens/` 确认变量名与取值格式（HSL 三元组）。强调色与圆角不写死，取 `--primary` / `--radius`。
  - i18n 键：`shell.brandBadge`、`shell.searchPlaceholder`、`shell.adminGroup`、`shell.openMenu`、`shell.openSettings`、`shell.login`。

- [ ] **Step 1: 写失败测试**

  ```ts
  // MarvisSidebar.test.ts
  it("renders the member menu from workspaceMenus", () => { /* role user, gallery off */ expect(links()).toEqual(["/upload", "/images", "/albums", "/dashboard", "/tokens"]); });
  it("groups admin children under a labelled section", () => { /* role admin */ expect(w.text()).toContain("管理"); expect(links()).toContain("/admin/users"); });
  it("shows only gallery and a login button to anonymous visitors", () => { /* no user, gallery on */ expect(links()).toEqual(["/gallery"]); expect(w.find('[data-testid="sidebar-login"]').exists()).toBe(true); });
  it("marks the active route with aria-current", async () => { await router.push("/images"); expect(w.find('a[href="/images"]').attributes("aria-current")).toBe("page"); });
  it("hides labels when collapsed but keeps accessible names", () => { /* collapsed */ expect(w.find('a[href="/upload"]').attributes("aria-label")).toBeTruthy(); });
  // MarvisLayout.test.ts
  it("closes the mobile drawer after navigation", async () => { /* narrow=true; open; router.push */ expect(w.find('[data-testid="marvis-sidebar"]').classes()).not.toContain("is-open"); });
  it("follows sidebar.collapsed from Vben preferences", async () => { updatePreferences({ sidebar: { collapsed: true } }); await nextTick(); expect(w.findComponent(MarvisSidebar).props("collapsed")).toBe(true); });
  ```

- [ ] **Step 2: 跑测试确认失败** — `FE pnpm vitest run src/components/layout` → FAIL
- [ ] **Step 3: 实现**。布局尺寸照 Marvis 模板：侧栏 244px、外边距 14px、圆角 20px；收起时 68px。侧栏导航用 `<RouterLink>`。搜索入口只 `emit("open-search")`，设置按钮只 `emit("open-settings")`。
- [ ] **Step 4: 验证页面容器**。`FE pnpm dev -- --host`（或本机可用的方式起开发服务器），以 Marvis 外壳打开 `/images`、`/upload`、`/admin/users`，检查 `@vben/common-ui` 的 `Page` 高度与滚动是否正常。若异常：在 `MarvisLayout` 根节点上提供 `Page` 所读取的同名 CSS 变量（从 `vendor/vben/packages/effects/common-ui/src/components/page/` 查出变量名），不改任何页面。把结论写进提交信息。
- [ ] **Step 5: 跑测试确认通过** — 同 Step 2 → PASS
- [ ] **Step 6: Commit** — `feat(web-vben): add Marvis layout, sidebar and design tokens`

---

### Task 4: 侧栏相册分组与新建相册

**Files:**
- Create: `web-vben/src/components/layout/marvis/SidebarAlbums.vue`
- Create: `web-vben/src/components/layout/marvis/useSidebarAlbums.ts`
- Modify: `web-vben/src/components/layout/MarvisLayout.vue`（把 `SidebarAlbums` 放进侧栏 `#albums` 插槽）
- Modify: `shell.json`
- Test: `web-vben/src/components/layout/marvis/SidebarAlbums.test.ts`

**Interfaces:**
- Consumes: `listAlbums({ page, size })`、`AlbumView` from `@/api/albums`；`AlbumFormModal`（props `show`、`album`；emits `update:show`、`saved: [album: AlbumView]`）；`useAuthStore`。
- Produces:
  ```ts
  export const SIDEBAR_ALBUM_LIMIT = 20;
  export function useSidebarAlbums(): {
    albums: Readonly<ShallowRef<AlbumView[]>>; total: Readonly<ShallowRef<number>>;
    loading: Readonly<ShallowRef<boolean>>; failed: Readonly<ShallowRef<boolean>>;
    load: () => Promise<void>;
  };
  ```
  身份（`auth.user?.id`）变化时清空并重新加载；未登录不请求。请求用自增序号丢弃过期响应（与 `useAlbums` 同一写法）。
  - `SidebarAlbums.vue`：展开/收起；列出相册 → `/albums/:id`；末尾固定「新建相册」（打开 `AlbumFormModal`，`album=null`）与「全部相册」（→ `/albums`）。`total > albums.length` 时「全部相册」显示总数。保存成功后 `load()` 并 `router.push("/albums/" + album.id)`。
  - i18n 键：`shell.albums.create`、`shell.albums.all`、`shell.albums.loadFailed`、`shell.albums.retry`、`shell.albums.empty`。

- [ ] **Step 1: 写失败测试**（mock `@/api/albums`）

  ```ts
  it("lists albums as links once expanded", async () => { expect(hrefs()).toEqual(["/albums/1", "/albums/2"]); });
  it("always offers create and view-all entries, even with no albums", async () => { /* items: [] */ expect(w.find('[data-testid="album-create"]').exists()).toBe(true); expect(w.find('a[href="/albums"]').exists()).toBe(true); });
  it("shows the total on view-all when more albums exist than listed", async () => { /* total 57, 20 items */ expect(w.find('a[href="/albums"]').text()).toContain("57"); });
  it("shows an inline retry on failure without breaking the rest", async () => { /* listAlbums rejects */ await w.find('[data-testid="album-retry"]').trigger("click"); expect(listAlbums).toHaveBeenCalledTimes(2); });
  it("opens the existing album form and navigates to the new album after save", async () => { /* 触发 AlbumFormModal 的 saved({id: 9}) */ expect(listAlbums).toHaveBeenCalledTimes(2); expect(router.currentRoute.value.path).toBe("/albums/9"); });
  it("drops the previous account's albums when the identity changes", async () => { auth.user = otherUser; await flushPromises(); expect(w.text()).not.toContain("旧账号的相册"); });
  it("does not request albums for anonymous visitors", () => { expect(listAlbums).not.toHaveBeenCalled(); });
  it("ignores a stale response that resolves after a newer one", async () => { /* 两次 load，先 resolve 第二次 */ expect(names()).toEqual(["second"]); });
  ```

- [ ] **Step 2: 跑测试确认失败** — `FE pnpm vitest run src/components/layout/marvis/SidebarAlbums.test.ts` → FAIL
- [ ] **Step 3: 实现**
- [ ] **Step 4: 跑测试确认通过**；再跑 `FE pnpm vitest run src/views/AlbumsView.test.ts src/components/albums` 确认相册现有测试未受影响
- [ ] **Step 5: Commit** — `feat(web-vben): list and create albums from the Marvis sidebar`

---

### Task 5: `Ctrl K` 全局检索

**Files:**
- Create: `web-vben/src/components/layout/marvis/MarvisCommandPalette.vue`
- Modify: `web-vben/src/components/layout/MarvisLayout.vue`（挂载、接 `open-search`）
- Modify: `shell.json`
- Test: `web-vben/src/components/layout/marvis/MarvisCommandPalette.test.ts`

**Interfaces:**
- Consumes: `useSearchSuggestions({ raw, caret, composing, focused, scopeAlbumId })`、`defaultTimezone()` from `@/components/images/…`；`workspaceMenus`；`useAuthStore`。
- Produces: `defineModel<boolean>("show")`。回车时
  ```ts
  router.push({ path: "/images", query: { qv: "1", q: raw.trim(), tz: defaultTimezone(), page: "1", size: "20" } })
  ```
  （与 `useImageQueryState.writeRoute` 的参数名一致）。空输入回车 → 跳 `/images` 不带查询。弹窗不解析搜索语法。
  - 快捷键在 `MarvisLayout` 注册：`(ctrlKey || metaKey) && key === "k"`，`preventDefault`；`!auth.user` 时不注册。
  - 页面跳转项来自 `workspaceMenus()`，按输入做不区分大小写的标题包含匹配。
  - i18n 键：`shell.palette.placeholder`、`shell.palette.pages`、`shell.palette.searchImages`、`shell.palette.empty`。

- [ ] **Step 1: 写失败测试**

  ```ts
  it("opens on Ctrl+K and on Cmd+K, closes on Escape", async () => {});
  it("does not register the shortcut for anonymous visitors", async () => { /* 派发 keydown */ expect(palette().props("show")).toBe(false); });
  it("submits the raw query to /images using the unified-search route params", async () => { /* 输入 "album:旅行 cat" + Enter */ expect(router.currentRoute.value.query).toMatchObject({ qv: "1", q: "album:旅行 cat", page: "1", size: "20" }); });
  it("does not submit while an IME composition is active", async () => { /* compositionstart → Enter */ expect(router.currentRoute.value.path).not.toBe("/images"); });
  it("navigates to a matching page entry", async () => { /* 输入 "回收" 或对应菜单标题，点击 */ });
  it("returns focus to the trigger after closing", async () => {});
  it("applies a chosen suggestion to the input instead of navigating", async () => {});
  ```

- [ ] **Step 2: 跑测试确认失败** — `FE pnpm vitest run src/components/layout/marvis/MarvisCommandPalette.test.ts` → FAIL
- [ ] **Step 3: 实现**。用 `NModal`（自带焦点锁定与 `Esc`）；`role="dialog"` + `aria-label`。
- [ ] **Step 4: 跑测试确认通过**；再跑 `FE pnpm vitest run src/components/images` 确认搜索现有测试未受影响
- [ ] **Step 5: Commit** — `feat(web-vben): add Ctrl+K command palette to the Marvis shell`

---

### Task 6: 设置弹窗与外观面板

**Files:**
- Create: `web-vben/src/components/layout/marvis/MarvisSettingsModal.vue`
- Create: `web-vben/src/components/layout/marvis/AppearancePane.vue`
- Create: `web-vben/src/components/account/ProfileCard.vue`（从 `AccountSettingsView.vue` 抽出头像 + 显示名表单的 `NCard`，逻辑原样）
- Modify: `web-vben/src/views/AccountSettingsView.vue`（改用 `ProfileCard`；`Page` 外壳、标题、`ChangePasswordCard` 不变）
- Modify: `web-vben/src/components/layout/MarvisLayout.vue`、`shell.json`
- Test: `web-vben/src/components/layout/marvis/AppearancePane.test.ts`、`MarvisSettingsModal.test.ts`

**Interfaces:**
- Consumes: `shell`、`setShell`、`resetAppearance`、`SHELL_DEFAULTS`；`preferences`、`updatePreferences`；`ProfileCard`、`ChangePasswordCard`。
- Produces:
  ```ts
  // AppearancePane.vue —— 无 props
  export const ACCENT_PRESETS = [SHELL_DEFAULTS.marvis.colorPrimary, SHELL_DEFAULTS.classic.colorPrimary, "hsl(160 84% 34%)", "hsl(18 76% 53%)", "hsl(317 54% 49%)"] as const;
  export const RADIUS_PRESETS = ["0.25", "0.5", "0.75"] as const; // 小 / 中 / 大
  // MarvisSettingsModal.vue
  defineModel<boolean>("show");  // 分类：appearance | account | tokens
  ```
  - 各控件写入：布局 → `setShell`；深色模式（三态 light/dark/auto）→ `updatePreferences({ theme: { mode } })`；强调色 → `theme.colorPrimary`；圆角 → `theme.radius`；紧凑侧栏 → `sidebar.collapsed`；恢复默认 → `resetAppearance()`。
  - Token 分类只放说明和跳转 `/tokens` 的链接，点击后关闭弹窗。
  - ≤768px 全屏、分类横向滚动。
  - i18n 键：`shell.settings.title`、`shell.settings.appearance|account|tokens`、`shell.appearance.layout|layoutMarvis|layoutClassic|layoutMarvisHint|layoutClassicHint|mode|modeLight|modeDark|modeAuto|accent|radius|radiusSmall|radiusMedium|radiusLarge|compactSidebar|reset`、`shell.settings.tokensHint|tokensLink`。

- [ ] **Step 1: 写失败测试**

  ```ts
  // AppearancePane.test.ts
  it("switches to the classic layout", async () => { await click('[data-testid="layout-classic"]'); expect(shell.value).toBe("classic"); });
  it("writes the chosen accent to Vben preferences", async () => { await click('[data-testid="accent-2"]'); expect(preferences.theme.colorPrimary).toBe(ACCENT_PRESETS[2]); });
  it.each([["radius-0", "0.25"], ["radius-2", "0.75"]])("writes radius %s", async (id, v) => { await click(`[data-testid="${id}"]`); expect(preferences.theme.radius).toBe(v); });
  it("sets the theme mode", async () => { await click('[data-testid="mode-dark"]'); expect(preferences.theme.mode).toBe("dark"); });
  it("toggles the compact sidebar", async () => { await click('[data-testid="compact-sidebar"]'); expect(preferences.sidebar.collapsed).toBe(true); });
  it("reset restores the current shell defaults", async () => { updatePreferences({ theme: { colorPrimary: ACCENT_PRESETS[3], radius: "0.25" } }); await click('[data-testid="appearance-reset"]'); expect(preferences.theme).toMatchObject(SHELL_DEFAULTS.marvis); });
  it("marks the active preset with aria-pressed", () => {});
  // MarvisSettingsModal.test.ts
  it("shows the appearance pane first and switches panes", async () => {});
  it("closes itself when following the tokens link", async () => { expect(router.currentRoute.value.path).toBe("/tokens"); expect(w.emitted("update:show")?.at(-1)).toEqual([false]); });
  ```

- [ ] **Step 2: 跑测试确认失败** — `FE pnpm vitest run src/components/layout/marvis` → FAIL
- [ ] **Step 3: 抽 `ProfileCard` 并实现弹窗**。抽取后先跑 `FE pnpm vitest run src/views/AccountSettingsView.test.ts`，断言不改且通过，再继续。
- [ ] **Step 4: 跑测试确认通过** — 同 Step 2 → PASS
- [ ] **Step 5: Commit** — `feat(web-vben): add settings modal with appearance options`

---

### Task 7: 图片库右侧详情面板

**Files:**
- Create: `web-vben/src/components/images/ImageDetailContent.vue`（从 `ImageDetailDrawer.vue` 抽出：预览图、`NDescriptions`、EXIF 懒加载与 `requestId` 防过期逻辑）
- Create: `web-vben/src/components/images/ImageDetailPanel.vue`
- Modify: `web-vben/src/components/images/ImageDetailDrawer.vue`（只保留 `NDrawer` 外壳 + `ImageDetailContent`）
- Modify: `web-vben/src/components/images/ImageLibrary.vue`
- Modify: `web-vben/src/components/images/ImageCard.vue`（新增可选 prop `active?: boolean`，为真时加 `image-card--active` 类；新增可选 prop `selectOnClick?: boolean`，为真时点击卡片信息区（`image-card__body` 中非按钮、非开关的区域）`emit("open")`。缩略图点击仍然打开灯箱，文件名点击仍然 `emit("open")`，两者都不改）
- Test: `web-vben/src/components/images/ImageDetailPanel.test.ts`；扩充 `ImageLibrary.test.ts`

**Interfaces:**
- Consumes: `shell`；`useViewportBelow(1040)`（Task 3）；`lib.drawerShow`、`lib.drawerImage`、`lib.openDrawer`（不改 `useImageLibrary`）。
- Produces:
  ```ts
  // ImageDetailContent.vue
  defineProps<{ image: ImageView | null; active: boolean }>(); // active=false 时取消并清空 EXIF（等价于现抽屉的 show=false）
  // ImageDetailPanel.vue
  defineProps<{ image: ImageView | null }>(); defineEmits<{ close: [] }>();
  ```
  - `ImageLibrary`：`const usePanel = computed(() => shell.value === "marvis" && !narrow.value)`。`usePanel` 时根节点为两栏（内容 + 308px 面板），渲染 `ImageDetailPanel :image="lib.drawerShow.value ? lib.drawerImage.value : null"`；否则渲染现有 `ImageDetailDrawer`。两者不同时存在。
  - 右键菜单（`menuOptions`、`onMenuSelect`）、`NImageGroup`、搜索、批量栏、分页的代码不动。
  - `image.id` 相同且 `active` 持续为真时不重新请求 EXIF。

- [ ] **Step 1: 写失败测试**

  ```ts
  // ImageLibrary.test.ts 追加（现有用例一条不改）
  it("uses the side panel in the wide marvis shell", async () => { /* shell marvis, narrow false */ expect(w.findComponent(ImageDetailPanel).exists()).toBe(true); expect(w.findComponent(ImageDetailDrawer).exists()).toBe(false); });
  it.each([["classic", false], ["marvis", true]])("keeps the drawer for shell=%s narrow=%s", async () => { expect(w.findComponent(ImageDetailDrawer).exists()).toBe(true); expect(w.findComponent(ImageDetailPanel).exists()).toBe(false); });
  it("shows the same image after the viewport crosses 1040px", async () => { /* 打开详情 → narrow 置 true */ expect(w.findComponent(ImageDetailDrawer).props()).toMatchObject({ show: true, image: first }); });
  it("selects a card on click in panel mode and highlights it", async () => { expect(w.findComponent(ImageDetailPanel).props("image")).toEqual(first); expect(card(0).classes()).toContain("image-card--active"); });
  it("offers the same context-menu options in panel mode", async () => { /* 与现有右键用例同一组 key 断言 */ });
  // ImageDetailPanel.test.ts
  it("shows an empty state without an image", () => {});
  it("loads EXIF once per image and ignores a stale response", async () => { /* 先后设置 image A、B，A 的响应后到 */ expect(getImageExif).toHaveBeenCalledTimes(2); expect(w.text()).toContain("B 的相机型号"); });
  it("clears EXIF when the image is cleared", async () => {});
  ```

- [ ] **Step 2: 跑测试确认失败** — `FE pnpm vitest run src/components/images` → 新用例 FAIL，旧用例 PASS
- [ ] **Step 3: 先抽 `ImageDetailContent`**，跑 `FE pnpm vitest run src/components/images/ImageDetailDrawer.test.ts`，断言不改且通过
- [ ] **Step 4: 实现面板与 `ImageLibrary` 的分支**
- [ ] **Step 5: 跑测试确认通过** — `FE pnpm vitest run src/components/images src/views/ImagesView.test.ts src/views/AlbumDetailView.test.ts` → 全部 PASS
- [ ] **Step 6: Commit** — `feat(web-vben): show image details in a side panel under the Marvis shell`

---

### Task 8: Naive UI 贴合、文档与整体验收

**Files:**
- Modify: `web-vben/src/assets/marvis.css`（下拉菜单、弹窗、抽屉、分页、卡片、输入框的圆角与阴影；全部限定 `html[data-shell="marvis"]`）
- Modify: `docs/planning/dual-frontend-build.md`、`README.md`（「可选的 Vben Naive 前端」一节）
- Create: `docs/planning/marvis-shell-validation.md`（记录下面的验收结果）

- [ ] **Step 1: 全量自动检查**
  - `FE pnpm vitest run` → 全部通过
  - `FE pnpm typecheck` → 无错误
  - `FE pnpm lint` → 无错误
  - `FE pnpm build` → 成功；确认产物里 `ClassicLayout` 与 `MarvisLayout` 各自成独立 chunk
  - `docker compose -f deploy/compose.dev.yaml run --rm dev sh scripts/check-frontend-selection.sh` → `verified exclusive legacy/Vben dependency graphs…`
- [ ] **Step 2: 浏览器验收**（起后端与 `web-vben` 开发服务器；桌面宽度与 375px 各一遍；Marvis 与经典各一遍）
  1. 首次打开是 Marvis；设置 → 经典布局，界面与改动前一致；用户菜单 → 切回新版。全程不刷新、不掉登录。
  2. 上传一张图 → 「我的图片」可见。
  3. 统一搜索：输入条件、chips、建议、清空，行为与改动前一致。
  4. 右键图片 → 原图 / WebP / 缩略图 × URL / Markdown / HTML / BBCode 逐项复制，内容正确；属性、公开切换、删除确认可用。
  5. 点缩略图进灯箱，左右键翻页。
  6. `Ctrl K` 搜索并跳转；侧栏新建相册并进入。
  7. 外观：深浅色、五个强调色、三档圆角、紧凑侧栏、恢复默认；自选颜色后切换布局颜色不变。
  8. 管理后台六个页面逐页看一遍，记录哪些页面观感不够贴近（只记录，不在本计划内重写）。
- [ ] **Step 3: 按验收所见补 `marvis.css` 贴合规则**，重跑 Step 1 的 `vitest` 与 `build`
- [ ] **Step 4: 写文档**。`dual-frontend-build.md` 说明 `web-vben` 内含两套外壳、默认 Marvis、偏好存 `localStorage["imgnest-shell"]`；`README.md` 加一句；`marvis-shell-validation.md` 记录 Step 1 的命令输出摘要、Step 2 逐项结果与未验证项。
- [ ] **Step 5: Commit** — `docs: record Marvis shell build notes and validation`
- [ ] **Step 6: 合并前** 按 `AGENTS.md` 走 `verification-before-completion` 与 `requesting-code-review`。
