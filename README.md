# ImgNest

Go + Vue 3 自研图床项目。当前已实现 M1–M4：SQLite/PostgreSQL、用户与 Token、本机/S3 存储、同步图片上传、WebP、双缩略图、完整本地元数据、无损隐私清理、可恢复回收站，嵌入二进制的 Vue 前端（用户端 MVP + 管理后台），以及与蓝空（Lsky Pro）字段级兼容的 `/api/v1` 接口（PicGo/uPic 可直连，支持游客上传）。相册管理、公开画廊与蓝空数据迁移属于 M5。

运行与验证步骤见 [开发说明](docs/development.md)，接口契约见 [OpenAPI](docs/openapi.yaml)，本批记录见 [M4 progress](docs/planning/m4-progress.md)，前端阶段见 [M3 progress](docs/planning/m3-progress.md)，后端核心见 [M2 progress](docs/planning/m2-progress.md)。仓库保留锁定的版本矩阵、33 个 AI Skill 与许可证说明。

## 目录

```text
imgnest-dev-kit/
├─ cmd/imgnest/              # 服务、迁移、账户与存储/规则初始化 CLI
├─ internal/                 # 分层业务、存储、libvips、元数据与安全路径
├─ deploy/                   # 固定 Go/libvips 开发镜像、PG/MinIO 测试服务
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
2. 前后端依赖均已锁定；前端 `web/pnpm-lock.yaml` 已提交，命令通过 `make fe-*` / `make release` 在容器内执行。
3. 用 Claude Code 打开项目即可：`CLAUDE.md` 和 `.claude/skills/` 会被自动加载。
4. 其他 AI 工具读 `AGENTS.md`。如果工具从 `.agents/skills/` 读 Skill，复制一份过去即可：
   - macOS / Linux：`cp -r .claude/skills .agents/skills`
   - Windows：`Copy-Item -Recurse .claude\skills .agents\skills`

## 开工准备

开工前检查见 [开工审查](docs/planning/2026-10-04-readiness.md)，当前计划见 [M4 实施计划](docs/superpowers/plans/2026-10-04-m4-lsky-admin.md)。实际验证结果和已知范围在 M4 progress 中记录。

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

## 可选的 Vben Naive 前端

`web/` 保留 M5 原版前端；`web-vben/` 是独立的 Vben Naive 应用，覆盖用户和管理员页面，提供中英切换及实际数据概览。默认发布仍使用原版；`make release-vben` 构建带新版页面的二进制。新版应与本次包含验证码配置端点的后端一起构建，不能把新静态包直接当作旧后端的无条件替换。

`web-vben/` 自带两套可在运行时切换的外壳：默认的 Marvis 风格（漂浮侧栏、`Ctrl K` 检索、右侧详情面板）和原 Vben 经典布局，在「设置 → 外观」里切换。

构建与切换见 [双前端说明](docs/planning/dual-frontend-build.md)，验证码启用和旧版兼容边界见 [验证码说明](docs/captcha.md)，本次检查通过项及未完成的浏览器验收见 [迁移验证说明](docs/planning/vben-validation.md)。
