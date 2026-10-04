# ImgNest 开工审查

审查日期：2026-10-04。依据：[计划书](../spec.md)、[版本矩阵](../versions.md)、`AGENTS.md` 及两个项目自建 Skill。

本文件记录开工前的审查快照。用户随后确认 M1 并授权并行实施；当前真实进度见 [M1 记录](m1-progress.md)，操作说明见 [开发说明](../development.md)。下文“尚未实现”描述的是审查时状态。

## 结论与本次交付

架构、技术栈和产品目标足以拆分 M1。当前还是开发起步包，尚无 `go.mod`、业务代码、前端、迁移脚本、OpenAPI 或锁文件，不能宣称已能构建或运行。

用户已指定仓库为 [biliblihuorong/imgnest](https://github.com/biliblihuorong/imgnest)，Go module 使用 `github.com/biliblihuorong/imgnest`。本次交付审查记录与 [M1 实施计划](../superpowers/plans/2026-10-04-m1-foundation.md)。计划里的补充设计需在开工前审阅；未更改 `spec.md`、依赖锁定值或第三方 Skill。

## 已检查的环境与版本

| 项目 | 实测 | 执行安排 |
| --- | --- | --- |
| 本地 Git | 当前目录不是 Git 仓库 | 建立起步包基线，再进入 `feat/m1-foundation` |
| 远端 Git | `git ls-remote https://github.com/biliblihuorong/imgnest` 成功且无 refs | 目前无远端提交，初始化前再核对一次 |
| Go | `go1.26.5 windows/amd64` | 使用锁定的 Go 1.27.1 Linux 构建环境 |
| Node | 24.19.0 | M3 使用锁定的 Node 24.21.0 容器 |
| pnpm | 11.19.0 | M3 使用 pnpm 12.9.1 |
| golangci-lint | 2.4.0 | 校验与 CI 使用 2.14.0 |
| Docker | Desktop Linux 引擎 29.8.1、linux/amd64 可访问 | 优先用 Docker；不需要先升级宿主工具链 |
| WSL | Ubuntu-22.04、Debian 为 stopped；docker-desktop 为 running | 作为可选环境，未启动或安装软件 |

初次在沙箱内查询 Docker/WSL 出现权限错误；在获得工具层授权后，只读查询确认可用。沙箱内直接请求模块注册表出现 TLS 错误；授权后的 M1 注册表请求成功。不能把沙箱错误当成宿主网络或 Docker 故障。

核验程度如下，**元数据可取不等于安装和编译已经通过**：

- Go 官方模块代理：版本表的 19 个直接依赖（含四个 AWS SDK 模块）锁定版本声明均可取得。
- npm 官方注册表：版本表中的 21 个前端直接依赖及 pnpm 12.9.1 均可取得；检查的 Vue/Router/Pinia/Vite/Vitest/TypeScript/ESLint 关键 peer 范围相容。完整依赖求解、可选 peer 和原生构建仍由首次安装验证。
- Go 1.27.1、Node 24.21.0 的官方发布可查；vipsgen 1.3.11 的发布说明对应 libvips 8.18.6。
- 已读取 Go、Node、PostgreSQL 18.6、imagor-base 运行与开发镜像的 manifest，目标 linux/amd64 与 linux/arm64 均存在。未拉取镜像或编译 cgo。
- 四个 AWS SDK 模块的补充核验最初遇到进程启动权限错误；用户说明 Bitdefender 拦截并要求重试后，四个请求均 HTTP 200。没有修改安全软件设置，没有下载业务依赖，也未生成 `go.sum` 或 `pnpm-lock.yaml`。

官方参考：[Go 发布](https://go.dev/dl/)、[Node 24.21.0](https://nodejs.org/en/blog/release/v24.21.0)、[vipsgen 1.3.11](https://github.com/cshum/vipsgen/releases/tag/v1.3.11)、[golangci-lint 2.14.0](https://github.com/golangci/golangci-lint/releases/tag/v2.14.0)、[imagor 构建文件](https://github.com/cshum/imagor/blob/master/Dockerfile)。模块声明取自 `proxy.golang.org/<module>/@v/<version>.mod`，npm 声明取自 `registry.npmjs.org/<package>/<version>`。

版本表有两项说明需要校正，但不能据此私自调整依赖：

| 声明来源 | versions.md 备注 | 模块自身 go.mod 声明 |
| --- | --- | --- |
| gorm.io/driver/postgres v1.6.3 | pgx/v5 v5.11.0 | pgx/v5 v5.10.0 |
| gorm.io/driver/sqlite v1.6.0 | go-sqlite3 v1.14.52 | go-sqlite3 v1.14.22 |

它们是直接驱动要求的最低版本，并不必然等于整个 module 图最终选择的版本。首次 `go mod tidy` 后，以实际 `go.mod` / `go.sum` 校正备注；若希望主动钉住更高间接版本，必须明确记录理由。`imagemeta v1.1.0` 的模块声明确实要求 Go 1.27.0，因此不能用宿主 Go 1.26.5 替代锁定环境来验收整个项目。

## 按已有决定收敛的执行顺序

| 阶段 | 工作范围 | 验收条件 |
| --- | --- | --- |
| M1 | 工程与配置、数据库迁移、用户/Token、原生鉴权 API | SQLite + PostgreSQL 的初始化、登录、鉴权、吊销和重启流程可复现 |
| M2 | 本机/S3、路径模板、libvips、EXIF、双缩略图、上传补偿，以及基础删除/恢复/清理 | curl 上传取得可用链接；故障无孤儿对象；删除后原 URL 404；B2 按版本清理 |
| M3 | Vue 登录、上传、我的图片、Token 管理、回收站基本入口 | 前后端实际联调，移动端与暗色模式可用 |
| M4 | 蓝空 v1 API、管理员用户/组/存储/规则管理 | 黄金 JSON 与 PicGo 联调通过，管理员能配置真实可用规则 |
| M5 | 相册管理界面、公开画廊、游客上传等 P1 完善 | 开关、权限与限额可验证；蓝空相册 API 所需基础能力应在 M4 前具备 |
| M6 | 发布镜像、全量 CI、操作文档与打磨 | amd64/arm64 发布构建及完整 P0 验收通过 |
| v1 后续 | import-lsky、历史图片补处理、SQLite→PG 迁移命令 | 各自另写实施计划，避免纳入首版关键路径 |

这是对执行顺序的建议，不是对 `spec.md` 的替换：其中 **蓝空迁移延期** 已在功能表与末尾决定确认；**回收站 P0** 已在功能表确认。M2 要与上传共同实现基本回收站语义，不能等到 M5 才补删除行为。Docker 开发构建和基础测试在 M1 引入，M6 负责发布打磨，避免临发布才发现 cgo 环境问题。

## 需要在对应阶段前关闭的问题

### M1 开工前

1. 审阅 M1 计划的补充设计：默认关闭注册、显式初始化管理员、密码长度规则、web Token 有效期、错误码与改密接口。计划书要求相关能力，但未给出这些精确值。
2. 接受在 M1 先迁移 `groups/users/tokens/settings`，M2 再新增图片、规则、存储等表；未来新增版本化迁移，绝不回改已发布脚本。
3. 本地和远端 refs 核对后初始化 Git、设置 origin、建立 feature 分支。所有写远端动作都在实际交付时单独处理。

### M2 前：图片、存储与模型

| 问题 | 依据与影响 | 建议明确的契约 |
| --- | --- | --- |
| 容量口径 | 有原图/WebP/云端缩略图三份实体，Skill 只写 `used_bytes += size` | `used_bytes` 建议计实际云端对象字节之和，同 Key 只计一次；本地缩略图不计；回收站单列；原生原图大小与蓝空 KB 字段独立保留 |
| 规则字段缺失 | spec §6 的 `webp_effort/strip_meta/skip_if_larger`、§5 的冲突策略，以及 `heif_mode` 未完整进入 policies 字段表 | M2 迁移、配置 DTO、OpenAPI 和后台表单同时补齐；先决定是否允许关闭衍生图脱敏 |
| `_thumbs` 处理冲突 | spec §5 要重新生成，Skill 要追加 `-1`；确定性模板重新生成可能永远不变 | 在规范中分别定义随机模板重试与确定性文件名改名行为，再同步 Skill |
| 路径长度单位 | spec 全长 ≤255 未给单位，Skill 定为 255 bytes；清洗字符集合也不同 | 明确每段按 rune、全长按 UTF-8 bytes 的具体限制和拒绝/截断策略，纳入中文测试 |
| WebP 模式交叉 | `webp_only + skip_if_larger`；源文件本身为 WebP；HEIC 默认只存 WebP | 明确 only 模式不因变大丢失唯一文件；同一 WebP Key 不重复写入/计费/删除；返回字段描述实际保存版本 |
| 配额和路径并发 | 先检容量/路径再写对象，最后落库；并发请求可能同时通过预检或覆盖同 Key | 原子条件更新配额、在对象写入前取得路径占用，并保护补偿只删除本次写入；并发测试与事务失败测试覆盖 |
| 回收站部分失败 | 复制、删除对象及数据库事务分属不同系统 | 给 delete/restore/purge 定义幂等、补偿与失败状态；恢复也须检查配额并恢复容量，禁止双恢复/双扣费 |
| 脱敏失败回退 | §11 与 Skill 允许保留未脱敏文件，§6 的隐私目标要求云端移除敏感项 | 明确使用者能否接受默认 GPS 模式仍保存原文件；如改为拒绝上传，先同步规范与 Skill |
| 同步与异步冲突 | §6 返回前同步完成，§11 又建议 >30 MB 时先原图后后台 WebP | 首版建议遵守同步契约，用大小、像素和并发上限保护；异步需另定任务状态及客户端契约 |
| 私有图语义 | S3 桶/CDN 直链绕过应用，`is_public` 未规定对象访问控制 | 明确“私有”是否仅指不进入画廊/列表；若要求持有直链也必须鉴权，另设计私有桶/签名 URL/代理，不把画廊过滤称为对象保密 |

上述建议未直接改写规范。已有规范仍是事实来源；实现在这些边界开始前，把采纳的决定写进 `spec.md` 并同步项目 Skill。

### M4 前：精确兼容

- 固定蓝空源码 tag/commit，再生成请求样本和黄金 JSON；不能只依据示意 JSON 宣称兼容。
- 区分原生 RFC3339 时间与蓝空列表的 `Y-m-d H:i:s`；分页形状、空 `data`、HTTP 状态、HTML 转义、`album_id` 默认过滤需逐项核对。
- 框架与测试目录采用 spec 的 `internal/http/native`、`internal/http/lsky`；Skill 中 `internal/httpapi/lsky/testdata` 是旧目录建议，需要同步。
- 新旧 API 共用用户、Token、上传等 service；不复制第二套容量和鉴权逻辑。

## Skill 与工程约定的适用边界

- `golang-database` 倾向禁止 ORM、引入外部迁移工具，但项目已经明确 GORM、版本化 SQL、不引入未锁定依赖；项目规范优先。不能执行 Skill 的建议后悄悄更换技术栈。
- 通用 Skill 的 `@latest`、Viper、DI 工具、额外日志库建议不作为依赖授权；使用已经锁定的 koanf、手动注入和 slog。
- 现有 `executing-plans` 自带 task-start/task-done，但引用的 `using-git-worktrees`、`subagent-driven-development`、sdd-workspace/review-package 未包含在本起步包，在本机 `.codex/skills`、`.agents/skills` 中也未找到。不能直接执行依赖缺失的脚本；建议明确采用当前环境的原生 TDD、进度记录和末尾审查流程，不另装整套工作流。
- `context.Context` 在新设计的 Go 对外函数中置首；`main`、Gin handler、`http.Handler` 等固定框架签名保持库契约，业务操作传入 request context。
- 当前无业务代码，故本次没有运行项目测试、lint 或产品构建。M1 计划中的测试命令是未来验收要求，不能将它们的预期输出视为本次验证结果。

## 审查验证记录

- 已完整读取计划书、版本矩阵、两个项目 Skill 与蓝空/EXIF 引用契约。
- 已列出文件清单并验证本地无 Git 元数据、无业务脚手架。
- 已实测工具版本、Docker Linux 引擎、WSL 状态、关键官方发布/注册表与镜像 manifest。
- M1 计划自审要求：任务间签名一致、测试名与断言明确、依赖已锁定、补充设计与既有规范分开、阶段验收不包含尚未开发的 M2/M3 能力。
