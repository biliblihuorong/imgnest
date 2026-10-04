# ImgNest

Go + Vue 3 自研图床项目。当前已实现 M1 后端基础：配置、SQLite/PostgreSQL 版本化迁移、用户与 Token 鉴权、原生 API 和 CLI。图片上传、WebP 与 Vue 页面在后续里程碑实现。

运行与验证步骤见 [开发说明](docs/development.md)，接口契约见 [OpenAPI](docs/openapi.yaml)，本批记录见 [M1 progress](docs/planning/m1-progress.md)。仓库保留锁定的版本矩阵、33 个 AI Skill 与许可证说明。

## 目录

```text
imgnest-dev-kit/
├─ cmd/imgnest/              # serve / migrate / init-admin / reset-password
├─ internal/                 # config / model / repo / migrate / service / http / cli
├─ deploy/                   # Go 开发容器、PG 测试服务、配置示例
├─ go.mod / go.sum           # 已验证的后端依赖
├─ AGENTS.md                  # AI 协作说明（Codex、Cursor 等通用）
├─ CLAUDE.md                  # Claude Code 入口，引用 AGENTS.md
├─ docs/
│  ├─ spec.md                 # 计划书与开发规范（唯一事实来源）
│  └─ versions.md             # 工具链与依赖版本矩阵 + 可直接粘贴的 go.mod / package.json 片段
├─ .claude/skills/            # 33 个 Skill，Claude Code 自动发现
│  ├─ imgnest-image-pipeline/ # 项目自建：上传、转码、缩略图、EXIF 脱敏、回收站
│  ├─ lsky-api-compat/        # 项目自建：蓝空 v1 API 字段契约
│  └─ …                       # 31 个开源 Skill（Go / Vue / 测试 / 安全 / 部署 / 流程）
├─ skills-lock.json           # 每个 Skill 的来源仓库、路径、提交
├─ THIRD_PARTY_NOTICES.md     # 来源与许可证清单
├─ third_party/skills-licenses/  # 各仓库许可证全文
└─ scripts/
   ├─ install-skills.sh       # 按 lock 重新拉取 Skill（--latest 拉最新）
   └─ install-skills.ps1      # Windows 版（-Latest）
```

## 怎么用

1. 使用 Docker Desktop Linux 引擎，按 [开发说明](docs/development.md) 初始化、创建管理员并启动。
2. 后端依赖已锁定；后续前端按 `docs/versions.md` 安装并提交 `pnpm-lock.yaml`。
3. 用 Claude Code 打开项目即可：`CLAUDE.md` 和 `.claude/skills/` 会被自动加载。
4. 其他 AI 工具读 `AGENTS.md`。如果工具从 `.agents/skills/` 读 Skill，复制一份过去即可：
   - macOS / Linux：`cp -r .claude/skills .agents/skills`
   - Windows：`Copy-Item -Recurse .claude\skills .agents\skills`

## 开工准备

开工前检查见 [开工审查](docs/planning/2026-10-04-readiness.md)，已确认的任务见 [M1 实施计划](docs/superpowers/plans/2026-10-04-m1-foundation.md)。当前进度与实际验证结果在 M1 progress 中记录。

## 更新 Skill

```bash
./scripts/install-skills.sh            # 按 skills-lock.json 的提交重装（可复现）
./scripts/install-skills.sh --latest   # 拉各仓库最新版，看 diff 后再更新 lock 和 NOTICES
```

Windows：`./scripts/install-skills.ps1`（或加 `-Latest`）。两个项目自建 Skill 不会被覆盖。

## 在线文档

规范的在线可编辑版本：https://claude.ai/code/artifact/a63820dd-642d-4916-bdd0-706cdfbd1fd8
本包里的 `docs/spec.md` 是 2026-10-04 的导出快照，两张图已改为 Mermaid 图和表格。以后改规范请两边同步，或以仓库里的版本为准。

## 许可证

31 个开源 Skill 保留原许可证（MIT、Apache-2.0、CC-BY-SA-4.0），详见 `THIRD_PARTY_NOTICES.md`。
