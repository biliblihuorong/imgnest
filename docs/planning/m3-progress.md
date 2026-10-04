# M3 实施与验收记录

计划：[M3 frontend MVP](../superpowers/plans/2026-10-04-m3-frontend-mvp.md)、设计补充：[M3 design](../superpowers/specs/2026-10-04-m3-frontend-mvp-design.md)。基线为 M2 整合提交 `024bb04`（+ docs `3b35948`）；分支 `feat/m3-frontend-mvp` 叠加在 `feat/m2-image-core` 之上。

用户于2026-10-04确认：继续 M3、评估并行可行性并在可行时并行执行；随后追加授权推送远程仓库。并行评估结论：可行——后端仅 site/policies 两个小缺口（与前端零文件交集），四个页面按「脚手架先行 + stub 独占替换」三路并行。

## 交付范围

| 计划任务 | 实际结果 |
| --- | --- |
| 1 Node 工具链 | dev 镜像装入 node24.21.0 + pnpm12.9.1（npm i -g 固定），pnpm store 进命名卷，Makefile fe-* 与 release 目标，dist/.gitkeep 保底嵌入 |
| 2 后端补缺 | GET /api/site（公开，恰2字段不泄漏 settings）与 GET /api/policies（组绑定+规则启用+存储启用，与 M2 permittedPolicy 同源），双库契约测试，openapi 同步 |
| 3 前端脚手架 | 锁定版本精确安装 + pnpm-lock 入库；openapi-typescript 生成类型；client（envelope/ApiError/20001 会话重置钩子）；auth/site stores；5 路由 + 守卫 + redirect 回跳；Naive UI 布局（跟随系统暗色）；Login/Register + 3 个 stub |
| 4 上传页 | 拖拽/粘贴/多选预检（20文件/20MiB/64MiB）；XHR 逐文件字节进度；规则下拉（空规则引导/唯一自动选中）；单201/批量207 逐项呈现；URL/Markdown/HTML/BBCode × 原图/WebP/缩略图复制与缺失禁用；失败项单独重试 |
| 5 图片与回收站 | 网格分页（20/50/100）+ 本地缩略图占位；详情抽屉懒加载 EXIF（GPS 标注仅本人可见，raw 折叠）；可见性开关失败回滚；单图删除；回收站页签：单行/批量恢复与彻底删除、剩余保留时间展示 |
| 6 Token 与账户 | 列表（永不过期/从未使用展示）、创建（明文一次性弹窗+复制+警示）、吊销确认；改密卡（12–72字节预检，成功清态跳登录，20002 就地报错不登出） |
| 7 整合 | web/embed.go + router NoRoute SPA 回退（/api /i /t /healthz 保持 JSON 404；assets immutable；index no-store）；真实二进制冒烟通过 |
| 8 文档与审查 | development/README/versions/本记录同步；独立审查 7 项全过、无 P1 |

## 验证证据

全部在 Docker Linux amd64 容器内执行；PG/MinIO 实际运行，未用 skip 代替。

- 前端全链：`cd web && pnpm install --frozen-lockfile && pnpm gen:api && pnpm vitest run && pnpm typecheck && pnpm lint && pnpm build` exit0 —— **20 个测试文件 148 个用例全过**，vue-tsc 双 tsconfig 零错误，eslint 零错误，rolldown-vite 构建成功。
- Go 全量：`go test ./... -count=1` exit0（中途 PG/MinIO 因 Docker Desktop 守护进程抖动失败一次，重启容器后原样复跑全过，且 MinIO 用例在故障时是 FAIL 不是 SKIP，证明无跳过逻辑）；`go test -race ./internal/http` exit0；golangci-lint@v2.14.0 `./...` 0 issues；`go build -trimpath` 真实嵌入二进制。
- 真实冒烟（SQLite + 本机存储 + 真实二进制 + curl）：migrate/init-admin(stdin)/init-local → serve → healthz、`/` 返回真实 index.html、`/images` 深链回退（text/html, no-store）、`/assets/index-*.js`（immutable）、`/api/nope` JSON 404、`/api/site` 公开、登录、`/api/policies` 返回组规则、上传 201（1x1 PNG，links 正确、webp 因 skip_if_larger 为空符合预期）、列表、`/i/...` 直链 200 image/png、`/t/...` 200 image/webp。
- 并发协调：阶段1（后端 ∥ 脚手架）与阶段2（上传 ∥ 图片 ∥ Token）各 worker 测试独立通过后，父代理跑全量链把关；共享文件（client/stores/router/App）阶段2只读。

## 独立审查与修复

跨作者只读审查覆盖 7 项 Review Focus：token 单次展示、双通道 401 语义一致、207 逐项呈现、权限文案语义、守卫回跳、SPA 防穿越与保留前缀、site/policies 契约与授予语义、XSS 面（v-html/innerHTML/console 全仓 0 命中）。结论无 P1。

- P2：文档收尾缺失 → 本批 docs 提交补齐。
- P3 已修复：main.ts 会话重置注释改为仅 20001；types.ts 的 SiteInfo/PolicySummary 改由生成 schema 派生（openapi 已收录 SiteView/PolicySummary）；linkText HTML 复制文本最小转义（& < > "）；RegisterView 密码补 72 字节预检。
- P3 记录不改：vite.config 别名在 Windows 宿主直跑失准（锁外依赖 @types/node 不可引入，容器为验收环境，见 development.md 已知限制）；预检限额为硬编码默认值（部署调低限额时表现为预检放过、服务端 413 逐项失败，可解释降级，M4 可由配置下发）；上传为顺序逐文件（服务端并发默认 2，MVP 取保守）。

## 决定与实际限制

- 会话重置只在业务码 20001（过期/吊销/用户禁用）触发；20002 凭证内容错误就地展示——改密错旧密码不会被登出（审查确认该语义有反向测试）。
- Token 创建不提交 kind 字段：openapi `CreateTokenRequest` 明确 M1 服务端固定 api，前端展示固定标签；如需可选 kind 需 M4 扩契约。
- policies 列表语义为「组绑定 + 规则启用 + 存储启用」（比字面"仅规则启用"更严格，与上传 Preflight 授予完全一致，避免下拉里出现必然 403 的规则）。
- web token 存 `localStorage["imgnest.token"]`（Bearer+SPA 标准取舍）；前端不渲染用户 HTML，XSS 面靠文本插值控制。
- 图片网格未做多选批量操作（后端 /api/images/batch 已有，UI 留 M4）；上传未做并发池；回收站批量已覆盖。
- Playwright E2E、画廊/相册/游客上传、管理后台分别延后至 M4/M5；真实浏览器走查以冒烟 + 组件测试代替本轮自动化截图回归。
- 分支叠加在未合并的 M2 之上；两者均未合并 main（推送经用户追加授权）。

## 提交记录

`11d3149` docs 计划与设计 → `f422717` chore Node 工具链 → `bce4d64` chore pnpm 卷与 embed 占位 → `2b156e4` feat site/policies → `42ba574` feat 前端脚手架 → `c28e4eb` fix 会话重置收窄至 20001 → `bd70ffb` feat 上传页 → `238edb3` feat 图片网格与回收站 → `092a81b` feat Token/账户 → `cc36275` feat embed SPA 回退 → `43c81ca` fix 审查打磨 → 本 docs 提交同步交付记录。均未打 tag；M4 计划从本基线开始。
