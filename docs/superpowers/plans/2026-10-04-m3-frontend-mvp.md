# ImgNest M3 Frontend MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. 沿用 M2 的并行分工、契约先行、RED→GREEN TDD 与末尾独立审查；workers 不执行任何 Git 写操作，提交由父代理统一完成。

**Goal:** 交付 Vue 3 前端 MVP（登录/注册、上传、我的图片+回收站、Token/账户管理），构建产物嵌入 Go 二进制并 SPA 回退，vitest/vue-tsc/eslint 全绿，真实二进制冒烟通过。

**Architecture:** 前端只消费已冻结的 native API 契约（`docs/openapi.yaml`）；新增 `GET /api/site`、`GET /api/policies` 两个最小接口按本文契约先行冻结。Pinia 只放 auth/site 全局状态，请求只写在 `src/api/`，路由守卫做本地检查、鉴权由后端保证。构建产物 `web/dist` 经 `go:embed` 进入二进制，NoRoute 回退 SPA。

**Tech Stack:** 版本严格按 [versions.md](../../versions.md)：vue 3.5.43、vue-router 5.3.1、pinia 4.0.3、naive-ui 2.45.3、@vueuse/core 15.0.0、vite 8.3.2、typescript 6.0.3、vue-tsc 3.3.12、vitest 5.0.3、@vue/test-utils 2.5.1、jsdom 30.1.1、openapi-typescript 7.13.0、eslint 10.12.0 全家桶、prettier 3.9.9。Node 24.21.0 / pnpm 12.9.1 进 dev 镜像；Go 侧依赖不变。

**Spec:** [spec.md](../../spec.md) §7.2/§8/§10、[M3设计补充](../specs/2026-10-04-m3-frontend-mvp-design.md)、[versions.md](../../versions.md)。基线为 M2 整合提交 `024bb04`（+ docs `3b35948`）。分支 `feat/m3-frontend-mvp`，叠加在未合并的 `feat/m2-image-core` 之上。

**Status:** Tasks 1–8 已完成：实现、全量验证与独立审查（无 P1；P2 文档收尾与本批提交，P3 已修复或记录为已知限制）见 [M3 progress](../../planning/m3-progress.md)。原始 checkbox 保留为设计/验收依据。

## Global Constraints

- 依赖版本逐个精确锁定，不加 unplugin/axios/组件库之外的新依赖；确需新增先改 versions.md 并说明。
- 前端构建、测试、lint 全部在 dev 容器内执行；宿主 Node 仅作手工开发便利，不作验收依据。
- 请求只写在 `src/api/`；组件不直接 fetch；全局状态只有 auth/site 两个 store。
- 不渲染任何用户提供的 HTML（XSS）；Token 只进 `localStorage["imgnest.token"]`，不进日志、不进 URL。
- 后端接口契约以 `docs/openapi.yaml` 为唯一事实；前端发现的契约缺口先报告，不私自改 Go 代码。
- workers 文件边界见下表，不改他人目录、不改锁文件、不执行 Git 写命令。
- Go 侧改动沿用 M2 分层与测试要求；0001/0002 迁移不动，本轮无新迁移。
- 不引入路由级代码分割以外的构建花活；不做 SSR；不做 i18n 框架（文案直接中文）。

## Review Focus

1. Token 与错误处理：401/20001/20002 必须清态跳登录；明文 token 只出现一次（创建响应），不进日志。
2. 上传补偿的用户可见性：部分失败（207）逐项呈现，成功项可复制、失败项可重试，不把 207 当全成功。
3. 权限展示正确：EXIF/GPS 仅本人（M3 无 admin 前端）；私有图仅列表隐藏的语义在 UI 上不误导为链接失效。
4. 守卫与刷新：深链刷新（/images 直达）在无 token 时回登录且登录后回跳原路径。
5. 嵌入安全：静态服务不得暴露 web/ 源文件或物理存储对象；/api /i /t /healthz 不被 SPA 回退吞掉。

## 文件边界与并行顺序

| 阶段 | 工作区 | 文件 |
| --- | --- | --- |
| 父代理先行 | deploy、Makefile、.gitignore、docs/planning | Node 进镜像、pnpm 卷、fe 目标、dist/.gitkeep |
| 阶段1 BE worker | internal/service、internal/repo、internal/http/native、internal/http/router.go、docs/openapi.yaml | site/policies 接口 + openapi 同步（独占 Go 侧与 openapi） |
| 阶段1 FE-infra worker | web/**（全部脚手架）、.nvmrc | 脚手架、api client、stores、router+守卫、布局、Login/Register、全部页面 stub |
| 阶段2 上传 worker | web/src/views/UploadView.vue、web/src/api/upload.ts、web/src/api/policies.ts、web/src/components/upload/** | 上传页 + 规则下拉数据 |
| 阶段2 图片 worker | web/src/views/ImagesView.vue、web/src/views/TrashView.vue、web/src/api/images.ts、web/src/components/images/** | 网格、详情/EXIF 抽屉、可见性、回收站 |
| 阶段2 Token worker | web/src/views/TokensView.vue、web/src/api/tokens.ts、web/src/api/auth.ts 补充、web/src/components/account/** | Token CRUD、修改密码对话框 |
| 父代理整合 | web/embed.go、internal/http/router.go、Makefile、docs、progress | embed、SPA 回退、全量验证、审查、提交 |

阶段1两路并行；阶段2三路并行（基于已提交的脚手架，stub 由各自 worker 独占替换）；共享文件（client/stores/router/App）在阶段2只读，需要改动时报告父代理。

## Shared Interfaces

HTTP 契约（新接口，两侧共同实现；外壳与错误码沿用 M1/M2）：

- `GET /api/site`（公开）→ `data: {"site_name": string, "register_enabled": boolean}`。
- `GET /api/policies`（Bearer）→ `data: [{"id": <ID>, "name": string}]`；空为 `[]`；401 沿用 20001。

前端内部契约（FE-infra worker 冻结，阶段2 workers 消费）：

| 模块 | 导出 |
| --- | --- |
| `src/api/client` | `request<T>(path, init): Promise<T>`（自动 Bearer、解 envelope、非 0 抛 `ApiError{code,message,status}`）；`ApiError` 类型；401 处理钩子 |
| `src/api/auth` | `login(email,password)`、`register(...)`、`logout()`、`me()`、`changePassword(cur,next)` |
| `src/api/site` | `fetchSite(): Promise<SiteInfo>` |
| `src/stores/auth` | `token: string \| null`、`user: UserView \| null`、`login()`/`logout()`/`restore()`（启动时恢复 token+me） |
| `src/stores/site` | `siteName`、`registerEnabled`、`ensureLoaded()` |
| `src/router` | 上述 5 路由 + 守卫；`ROUTER_NAMES` 常量 |
| `src/lib/format` | `formatBytes(n: number): string`（KB/MB 自适应，中文单位） |

各页面 worker 在此之上增加自己的 `src/api/<域>.ts`，不改动共享模块签名。

## Task 1: Node 工具链进入 dev 镜像（父代理）

**Files:** deploy/Dockerfile.dev、deploy/compose.dev.yaml、Makefile、.gitignore、web/dist/.gitkeep。

- [ ] Dockerfile.dev 增加 node:24.21.0-bookworm 阶段（node+npm+全局 pnpm@12.9.1），不覆盖 Go/vips 路径；compose 增加 pnpm-store 卷与 `npm_config_store_dir`。
- [ ] Makefile 增加 `fe-install`、`fe-test`、`fe-lint`、`fe-build`、`release`（fe-build+go build）目标。
- [ ] .gitignore 增加 `web/node_modules/`、`web/dist/*`、`!web/dist/.gitkeep`、`web/coverage/`；创建 `web/dist/.gitkeep`。
- [ ] Run `docker compose -f deploy/compose.dev.yaml build dev`；Expected exit0；容器内 `node -v`=24.21.0、`pnpm -v`=12.9.1、`go version`=1.27.1。
- [ ] 提交（父代理）`chore: add node toolchain to dev image`，随后建分支 `feat/m3-frontend-mvp`。

## Task 2: 后端 site/policies 最小接口（BE worker，与 Task 3 并行）

**Files:** internal/service/site.go（或并入现有 service 文件）、internal/repo/policy.go、internal/http/native/site.go、internal/http/native/policy.go、internal/http/router.go、docs/openapi.yaml、对应 _test.go。

- [ ] 先写 RED：site 公开 200 且不泄漏其他 settings、`GET /api/policies` 未登录 401/20001、仅返回本人组绑定的启用规则、空组 `data=[]`、错误码映射正确。
- [ ] service 只依赖 repo 接口；规则可见性复用 M2 组授予逻辑；context 首参、%w 包装。
- [ ] openapi.yaml 增加 paths + SiteView/PolicySummary schema 与 envelope，保持 $ref 全部本地可解析。
- [ ] Run `docker compose -f deploy/compose.dev.yaml up -d --wait postgres` 后 `docker compose -f deploy/compose.dev.yaml run --rm dev go test ./internal/http/... ./internal/service ./internal/repo -count=1`；Expected RED→GREEN 全绿。
- [ ] 不执行 Git 命令；报告改动清单与测试证据，由父代理提交 `feat: expose site and policy listings`。

## Task 3: 前端脚手架与鉴权骨架（FE-infra worker，与 Task 2 并行）

**Files:** web/**（package.json、vite.config.ts、tsconfig、eslint.config、.prettier、vitest 配置、index.html、src/{api,stores,router,lib,views,components}）、.nvmrc。

- [ ] package.json 精确锁定 versions.md 全部前端版本 + `packageManager: pnpm@12.9.1`；`pnpm install`（容器内）成功且 lockfile 入库。
- [ ] `openapi-typescript` 生成 `src/api/schema.d.ts`（npm script `gen:api`）；`src/api/client.ts` envelope 解包/错误映射/401 钩子，先写失败测试再实现。
- [ ] auth/site stores + router 5 路由 + 守卫（未登录跳 /login 携带 redirect；已登录访问登录页跳 /upload）；启动 `restore()` 用 `me()` 校验 token。
- [ ] App 布局：NConfigProvider（跟随系统暗色）、菜单导航（上传/图片/Token）、用户下拉（修改密码入口、退出）；Login/Register 视图（register_enabled 控制注册入口）。
- [ ] 三个页面 stub（UploadView/ImagesView/TokensView 极简占位）供阶段2替换；`formatBytes` helper。
- [ ] 组件测试：client（成功/错误码/401）、auth store（login/logout/restore 失败）、守卫（未登录/已登录/redirect 回跳）、Login 表单 happy path。
- [ ] Run（容器内）`pnpm install --frozen-lockfile && pnpm gen:api && pnpm vitest run && pnpm typecheck && pnpm lint`；Expected exit0。
- [ ] 不执行 Git 命令；报告文件清单与证据，由父代理提交 `feat: scaffold vue frontend with auth shell`。

## Task 4: 上传页（upload worker，与 Task 5/6 并行）

**Files:** web/src/views/UploadView.vue、web/src/api/upload.ts、web/src/api/policies.ts、web/src/components/upload/**、对应测试。

- [ ] 拖拽/粘贴/多选（≤20 文件、单文件 20MiB、总 64MiB 客户端预检，超限提示不发请求）；policy 下拉（`GET /api/policies`，唯一规则自动选中，空规则给出引导文案）；公开/私有关闭开关。
- [ ] 逐文件进度与状态（排队/上传中/成功/失败）；单文件 201 与批量 207 分支都呈现，成功项展示原图/WebP/Markdown/HTML/BBCode 复制（`links` 驱动，webp 缺失时禁用切换）；失败项可重试。
- [ ] 组件测试：预检拒绝、201 成功复制字段、207 部分失败逐项、policy 空列表文案。`pnpm vitest run`（容器内）exit0。
- [ ] 替换 UploadView stub；不触碰共享文件；报告证据，父代理提交 `feat: build upload page with batch progress`。

## Task 5: 我的图片与回收站（images worker，与 Task 4/6 并行）

**Files:** web/src/views/ImagesView.vue、web/src/views/TrashView.vue、web/src/api/images.ts、web/src/components/images/**、对应测试。

- [ ] 网格分页（page/size≤100，`local_thumb_url` 展示，加载失败占位）；详情抽屉：元数据 + EXIF（含 GPS/raw，标注仅本人可见）；公开/私有关闭（PATCH）；单图删除（DELETE）确认后进回收站。
- [ ] 回收站视图：`GET /api/trash` 列表、恢复（restore）、彻底删除（purge）二次确认；显示 purge_at 剩余天数。
- [ ] 组件测试：分页加载、可见性切换调用、删除→回收站流转、恢复/彻底删除确认。`pnpm vitest run` exit0。
- [ ] 替换两个 stub；不触碰共享文件；报告证据，父代理提交 `feat: build image grid with trash management`。

## Task 6: Token 与账户（token worker，与 Task 4/5 并行）

**Files:** web/src/views/TokensView.vue、web/src/api/tokens.ts、web/src/api/auth.ts 补充、web/src/components/account/**、对应测试。

- [ ] Token 列表（TokenView 字段：kind/abilities/expires_at/last_used_at）；创建表单（name、kind=api、可选过期时间）；明文 token 一次性展示 + 复制 + 「关闭后不可再看」警示；吊销确认。
- [ ] 修改密码对话框（current/new，成功后强制重新登录）；退出登录清态。
- [ ] 组件测试：创建返回明文仅一次、吊销确认、改密成功后跳登录。`pnpm vitest run` exit0。
- [ ] 替换 TokensView stub；不触碰共享文件；报告证据，父代理提交 `feat: build token and account management`。

## Task 7: 嵌入、回退与全量验证（父代理整合）

**Files:** web/embed.go、internal/http/router.go、Makefile、docs/development.md、README.md、docs/planning/m3-progress.md。

- [ ] `web/embed.go` + `dist/.gitkeep` 使无构建 Go 可编译；`pnpm fe build` 产物进入二进制；NoRoute：/api /i /t /healthz 保持 JSON 404，/assets immutable 缓存，其余回退 index.html（no-store）。
- [ ] Run 全量：容器内 `pnpm vitest run && pnpm typecheck && pnpm lint && pnpm build`；`go test ./... -count=1`；`golangci-lint run`；`go build` 真实二进制。
- [ ] 真实冒烟：启动二进制（SQLite+本机存储），curl 首页 HTML、/assets、SPA 深链 /images 回退 index.html、/api 未知路径 JSON 404；登录→上传→列表→回收站一轮通过。
- [ ] 一次真实浏览器走查（视觉/交互/移动端宽度）。
- [ ] 独立审查 worker 全分支复查 Review Focus 五项；重要问题 RED→GREEN 修复后全绿。

## Task 8: 文档同步与收尾（父代理）

- [ ] development.md 前端开发段落（容器内 pnpm 流程）、README 运行说明、versions.md 执行补充记录。
- [ ] 新建 docs/planning/m3-progress.md 记录交付与证据；Conventional Commits 分任务提交；不推送/合并/tag。

## 当前交接

完成后进入 M4（蓝空 v1 兼容 API 与管理后台）。本轮不宣称画廊/相册/游客上传完成（M5）。
