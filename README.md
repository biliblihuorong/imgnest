# ImgNest 开发套件

ImgNest 图床项目的开发起步包：规范文档 + 锁定的版本矩阵 + 33 个 AI Skill（31 个开源 + 2 个项目自建）+ AI 协作说明。不含业务代码和脚手架。

## 目录

```text
imgnest-dev-kit/
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

1. 把整个目录的内容解压到项目仓库根目录（以后 `go.mod` 所在的位置）。
2. 按 `docs/versions.md` 搭脚手架。第一次 `pnpm install` / `go mod tidy` 后提交 `pnpm-lock.yaml` 和 `go.sum`。
3. 用 Claude Code 打开项目即可：`CLAUDE.md` 和 `.claude/skills/` 会被自动加载。
4. 其他 AI 工具读 `AGENTS.md`。如果工具从 `.agents/skills/` 读 Skill，复制一份过去即可：
   - macOS / Linux：`cp -r .claude/skills .agents/skills`
   - Windows：`Copy-Item -Recurse .claude\skills .agents\skills`

## 开工准备

开工审查和第一阶段任务已整理在 [开工审查](docs/planning/2026-10-04-readiness.md) 与 [M1 实施计划](docs/superpowers/plans/2026-10-04-m1-foundation.md)。它们记录了环境核验、规范冲突、待审阅的补充设计和验收要求；当前起步包尚未开始业务实现。

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
