# M3 前端 MVP 设计补充（2026-10-04）

计划：[M3 frontend MVP](../plans/2026-10-04-m3-frontend-mvp.md)。本文只记录计划书/规范未定死的实施决定；与 `docs/spec.md` 冲突时以 spec 为准。

## 范围

M3 交付：登录/注册、上传页、我的图片（含回收站管理）、Token 与账户管理，以及把构建产物 `web/dist` 以 `go:embed` 嵌入二进制并做 SPA 回退。相册、画廊、游客上传、管理后台、蓝空 v1 页面不在本轮（M4/M5）。Playwright E2E 延后到 M4 联调关口，本轮用 vitest 组件测试 + 真实二进制冒烟。

## 后端最小补缺（契约先行，写入 openapi.yaml）

| 接口 | 鉴权 | 响应 data | 用途 |
| --- | --- | --- | --- |
| `GET /api/site` | 公开 | `{"site_name": string, "register_enabled": boolean}` | 布局标题、登录页是否显示注册入口；不得泄漏其他 settings |
| `GET /api/policies` | 登录 | `[{"id": ID, "name": string}, …]`，空为 `[]` | 上传页规则下拉；返回当前用户组绑定的启用规则（组无绑定时为空），按 id 升序 |

两个接口都走现有 `{"code","message","data"}` 外壳与错误码分段；service 只依赖 repo 接口，规则可见性复用 M2 的组规则授予逻辑（UploadPolicy 同源），不新发明权限模型。

## 前端架构决定

- **工具链**：Node 24.21.0 / pnpm 12.9.1 进入 dev 镜像（`npm i -g pnpm@12.9.1`，不启用 corepack 交互）；宿主 Node 22 不作为验收依据。pnpm store 固定到容器卷。版本全部按 `docs/versions.md` 精确锁定，`.nvmrc` 写 `24.21.0`，`engines.node` `>=24.21.0 <25`。
- **类型生成**：`openapi-typescript@7.13.0` 从 `docs/openapi.yaml` 生成 `web/src/api/schema.d.ts`；`src/api/` 里手写薄封装（fetch + Bearer + envelope 解包 + 错误码映射），请求只写在 `src/api/`。
- **Token 保存**：web token（24h）存 `localStorage["imgnest.token"]`。这是 Bearer + SPA 架构下的标准取舍（规范未定 cookie）；XSS 面通过不渲染任何用户 HTML、依赖 Naive UI 文本插值控制。401/20001/20002 统一清 token 并跳登录。
- **全局状态**：Pinia 只放 `auth`（token + UserView）与 `site`（site_name、register_enabled）两个 store，列表数据留在页面。
- **路由**：`/login`、`/register`、`/upload`（默认首页，`/` 重定向）、`/images`、`/tokens`；全局守卫未登录跳 `/login`（携带 redirect 回跳），已登录访问 /login|/register 跳 `/upload`。守卫只做本地 token 存在性检查，真实校验靠首个 API 请求 401。
- **Naive UI**：直接按需 `import { NButton } from "naive-ui"`，不引入 unplugin 自动导入（不在锁定清单）；主题跟随系统（`useOsTheme` + darkTheme），移动端做基础响应式，不做专门移动端设计。
- **组件约定**：`<script setup lang="ts">`；组合函数 `useXxx`；页面组件 PascalCase 放 `src/views/`，复用组件放 `src/components/<域>/`；文案中文；字节数展示用统一 `formatBytes` helper。
- **测试**：vitest + @vue/test-utils + jsdom；client（envelope/错误映射/401）、两个 store、router 守卫、每个页面至少 happy path + 失败路径组件测试。覆盖率不设数字关口，按行为断言。

## 嵌入与回退

- `web/embed.go`（package web）`//go:embed all:dist` 导出 `Dist embed.FS`；`web/dist/.gitkeep` 入库保持无构建时 Go 可编译，`dist/*` 其余内容 gitignore。
- 集成时 `router.NoRoute`：`/api`、`/i`、`/t`、`/healthz` 前缀保持 JSON 404；其余路径回退 `dist/index.html`（no-store）；`/assets/**` 用文件服务带 `Cache-Control: public, max-age=31536000, immutable`（vite 已哈希）。不静态暴露 web/ 其他文件。

## 已知限制

- 上传进度用 XHR/fetch 的上传事件近似（单文件字节进度），不做分片续传。
- EXIF 详情含 GPS 与 raw，仅本人可见由后端保证，前端不做二次脱敏展示逻辑之外的控制。
- 回收站视图只做列表/恢复/彻底删除；「清空全站回收站」属管理员，延后。
- 首次真实浏览器走查（视觉/交互）在整合阶段做一次，不做自动化截图回归。
