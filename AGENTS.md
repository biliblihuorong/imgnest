# ImgNest — AI 协作说明

ImgNest 是用 Go + Vue 3 自研的图床，替代蓝空图床（Lsky Pro）社区版：多用户 × 多存储 × 多规则，原图 + WebP 双版本，兼容蓝空 v1 API。

## 先读这些

| 文件 | 内容 |
| --- | --- |
| `docs/spec.md` | 计划书与开发规范，**唯一事实来源**。与其他文件冲突时以它为准 |
| `docs/versions.md` | 锁定的工具链与依赖版本。不要擅自升级或引入新依赖 |
| `.claude/skills/imgnest-image-pipeline/` | 路径模板、WebP、缩略图、EXIF 脱敏、回收站的细则 |
| `.claude/skills/lsky-api-compat/` | 蓝空 v1 API 的精确字段契约 |

## 不可违反的规则

1. **分层单向**：`http → service → repo / storage / imaging`，禁止反向引用；service 只依赖接口。
2. **两套 API 共用同一套 service**：`/api/v1/*` 字段与蓝空完全一致（只多不少，单位不变：size 是 KB 浮点）；`/api/*` 是原生接口，外壳 `{"code","message","data"}`。
3. **上传必须可补偿**：任一对象写入或数据库事务失败，删除本次已写入的全部对象。
4. **格式只信文件头 + libvips 探测**，不信后缀和 Content-Type；默认拒绝 SVG；有像素上限。
5. **EXIF**：完整存本地库（含 GPS）；云端原图按 `scrub_mode` 无损抹除 GPS/序列号/作者/XMP，绝不重新编码；公开接口、画廊、蓝空 v1 接口一律不返回 EXIF。
6. **删除走回收站**（默认 7 天）：原 URL 立即 404；物理删除在 B2 上必须逐版本删除。
7. **数据库以 PostgreSQL + SQLite 为主**；表结构变更只能新增版本化迁移，不改已发布的迁移，不在生产路径依赖 `AutoMigrate`。
8. **libvips 版本与 vipsgen 包必须对应**（`vipsgen/vips` = 8.18.x），Dockerfile 里钉死。
9. 密钥、Token、密码、GPS 坐标不进日志。
10. 对象 Key 规则：`{path}.{ext}`、`{path}.webp`、`{path}_thumbs.webp`；`_thumbs` 是保留后缀；URL 不入库，实时拼接。

## 什么时候用哪个 Skill

| 场景 | Skill |
| --- | --- |
| 开始一个新功能、需求还模糊 | `brainstorming` → `writing-plans` → `executing-plans` |
| 写任何功能或修 bug | `test-driven-development`；遇到问题先 `systematic-debugging` |
| 宣布完成之前 | `verification-before-completion`；合并前 `requesting-code-review` |
| 上传、转码、存储、删除相关代码 | `imgnest-image-pipeline`（项目自建） |
| `/api/v1` 相关 | `lsky-api-compat`（项目自建） |
| Go 目录与风格 / lint | `golang-project-layout`、`golang-code-style`、`golang-lint` |
| 错误处理 / 数据库 / 并发 | `golang-error-handling`、`golang-database`、`golang-concurrency` |
| 安全 / 测试 / 性能 | `golang-security`、`golang-testing`、`golang-performance` |
| OpenAPI 文档 / CI | `golang-swagger`、`golang-continuous-integration` |
| Vue 组件、路由、Pinia、组件测试 | `vue-best-practices`、`vue-router-best-practices`、`vue-pinia-best-practices`、`vue-testing-best-practices` |
| Vite / Vitest / pnpm | `vite`、`vitest`、`pnpm` |
| 页面视觉 / 端到端测试 | `frontend-design`、`webapp-testing` |
| Dockerfile | `multi-stage-dockerfile` |
| PR 安全审查 / 静态扫描 | `differential-review`、`semgrep` |
| 新建或修改 Skill | `skill-creator` |

## 约定

- 提交信息用 Conventional Commits（`feat:` `fix:` `refactor:` `test:` `docs:` `chore:`）；`main` 永远可发布，功能走 `feat/*` 分支 + PR。
- Go：每个对外函数第一个参数是 `context.Context`；错误用 `%w` 包装，业务错误用哨兵值，handler 统一映射错误码（1xxxx 参数、2xxxx 鉴权、3xxxx 业务、5xxxx 存储/处理）。
- Vue：`<script setup lang="ts">` + Composition API；请求只写在 `src/api/`，类型由 `docs/openapi.yaml` 生成；全局状态只放用户信息与站点配置。
- 测试目标：`pathtpl`、`imaging`、`service/upload` 覆盖率 ≥ 80%；S3 用 MinIO 做集成测试；蓝空 API 用黄金 JSON 做契约测试。
- 改了规范、版本或 Skill，同步更新 `docs/` 与对应 `SKILL.md`。
