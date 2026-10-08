# ImgNest

自托管图床，后端 Go，界面 Vue 3，用来替代蓝空图床（Lsky Pro）社区版。图片放在你自己的服务器或存储桶里，PicGo、uPic、Typora 这些客户端改一下接口地址就能继续用。

> [!WARNING]
> ImgNest 仍在开发中，还没有发布 v1.0.0。Docker 镜像已经可以用，`edge` tag 跟随 `main` 分支的最新提交，可能不稳定。

## 特性

- **多用户 × 多存储 × 多规则**：用户组决定容量、单文件上限、可用的上传规则；每条规则绑定一个存储，存储支持本机磁盘和 S3 兼容服务（腾讯云 COS、Cloudflare R2、Backblaze B2 等）。
- **原图 + WebP 双版本**：上传时同步生成 WebP 和缩略图，响应返回时三个链接都已可访问；GIF 动图转动态 WebP，自动按 EXIF 纠正方向。
- **可配置的路径**：`{Y}/{m}/{d}/{uniqid}` 这类模板任意组合，对象命名固定为 `{path}.{ext}`、`{path}.webp`、`{path}_thumbs.webp`。
- **兼容蓝空 v1 API**：`/api/v1` 的路由、字段名、类型和单位与蓝空一致，只多不少；支持游客上传。
- **隐私**：完整 EXIF（含 GPS）只存在本地数据库；云端原图按规则无损抹除 GPS、序列号、作者和 XMP，不重新编码；公开接口和蓝空接口都不返回 EXIF。
- **回收站**：删除后原链接立即 404，默认保留 7 天可恢复，到期物理删除。
- **相册与公开画廊**：画廊可在后台开关。
- **一个镜像部署**：界面嵌在程序里；SQLite 和 PostgreSQL 共用同一个镜像，支持 linux/amd64 和 linux/arm64。
- **两种界面布局**：新版布局（浮动侧栏、`Ctrl K` 检索、详情面板）和经典布局可运行时切换，都支持深色模式和中英文。

## 快速开始

需要 Docker 和 Compose 插件。以下命令在仓库根目录执行，使用 SQLite：

```bash
git clone https://github.com/biliblihuorong/imgnest
cd imgnest

# 1. 启动（容器启动时会自动执行数据库迁移）
IMGNEST_TAG=edge docker compose -f deploy/compose.sqlite.yaml up -d

# 2. 创建管理员，密码从标准输入读取，长度 12–72 字节
read -rsp '管理员密码: ' password; echo
printf '%s' "$password" | docker compose -f deploy/compose.sqlite.yaml exec -T imgnest \
  imgnest init-admin --username admin --email admin@example.com
unset password

# 3. 创建本机存储和默认上传规则，--base-url 填别人访问图床时用的地址
docker compose -f deploy/compose.sqlite.yaml exec imgnest \
  imgnest init-local --base-url http://localhost:8080
```

打开 <http://localhost:8080> 登录并上传。注册默认关闭，管理员可以在站点设置里打开。

多用户场景改用 `deploy/compose.postgres.yaml`，启动前先设置 `POSTGRES_PASSWORD`。使用 S3 兼容存储时需要设置 `IMGNEST_SECURITY_MASTER_KEY`（32 字节随机数的 base64），它用来加密存储凭据，丢了就解不开，请单独备份。镜像 tag、数据卷、反向代理等见 [部署说明](docs/deployment.md)。

## 接入 PicGo / uPic / Typora

1. 在网页「账户与令牌」页面创建 Token，或者用邮箱密码换一个：

   ```bash
   curl -X POST https://img.example.com/api/v1/tokens \
     --data-urlencode 'email=you@example.com' \
     --data-urlencode 'password=你的密码'
   ```

2. PicGo 安装 `picgo-plugin-lankong`，接口地址填 `https://img.example.com/api/v1/upload`，鉴权选 Bearer，粘贴 Token。
3. uPic 选内置的蓝空图床；Typora 通过 PicGo 上传，填法相同。

字段说明见 [蓝空 API 与 PicGo](website/docs/guide/lsky-api.md)，完整契约见 [OpenAPI](docs/openapi.yaml)。

## 当前状态

| 已完成 | 内容 |
| --- | --- |
| 账户 | 用户、用户组、API Token、登录限流、验证码 |
| 存储 | 本机存储、S3 兼容存储、上传规则与路径模板 |
| 图片处理 | WebP、双缩略图、EXIF 归档与无损脱敏 |
| 管理 | 回收站、相册、公开画廊、管理后台、统一搜索 |
| 兼容 | 蓝空 v1 API、游客上传 |

还没做的：从蓝空导入用户、相册和图片记录（已延后）；历史图片的存量补处理；水印、审核 Webhook、AVIF 输出。完整需求和优先级见 [计划书与开发规范](docs/spec.md)。

## 技术栈

| 部分 | 选择 |
| --- | --- |
| 后端 | Go 1.27、Gin、GORM |
| 图片处理 | libvips 8.18，通过 vipsgen 调用；EXIF 读取用 imagemeta |
| 数据库 | SQLite 或 PostgreSQL，版本化迁移 |
| 对象存储 | aws-sdk-go-v2（S3 协议） |
| 界面 | Vue 3、Vite、TypeScript、Naive UI、Pinia，基于 Vben |

所有版本锁定在 [docs/versions.md](docs/versions.md)。

## 开发

开发环境跑在 Docker 的 Linux 引擎里，libvips 版本在镜像里钉死，宿主机不用装 Go 或 libvips。初始化、启动、测试的完整步骤见 [开发说明](docs/development.md)，常用命令：

```bash
make help       # 查看 imgnest 命令行帮助
make test       # Go 单元测试
make lint       # golangci-lint
make fe-test    # 前端 vitest
make fe-lint    # vue-tsc + eslint
make release    # 构建前端并输出嵌入界面的 bin/imgnest
```

动手前请先读：

- [docs/spec.md](docs/spec.md)：计划书与开发规范，唯一事实来源
- [AGENTS.md](AGENTS.md)：不可违反的规则和协作约定（分层、上传补偿、EXIF、回收站、对象 Key 等）
- 提交信息用 Conventional Commits，功能走分支 + PR，`main` 保持可发布

### AI 协作

仓库自带 33 个 Skill（`.claude/skills/`），其中 `imgnest-image-pipeline` 和 `lsky-api-compat` 是项目自建，其余来自开源仓库，来源和提交记录在 `skills-lock.json`。

- Claude Code 打开项目即可，`CLAUDE.md` 和 Skill 会自动加载。
- 其他工具读 `AGENTS.md`；如果工具从 `.agents/skills/` 读 Skill，复制一份过去：`cp -r .claude/skills .agents/skills`（Windows：`Copy-Item -Recurse .claude\skills .agents\skills`）。
- 按 lock 重装 Skill：`./scripts/install-skills.sh`；拉最新版加 `--latest`，看过 diff 再更新 lock 和 `THIRD_PARTY_NOTICES.md`。Windows 用 `./scripts/install-skills.ps1`。

## 目录

```text
imgnest/
├─ cmd/imgnest/        # 程序入口：serve、migrate、init-admin、reset-password、init-local、init-storage、init-policy
├─ internal/           # http → service → repo / storage / imaging 分层实现
├─ web-vben/           # 界面源码，构建后嵌入二进制
├─ website/            # 项目首页与文档站（VitePress）
├─ deploy/             # Dockerfile、生产与开发 compose、配置样例
├─ docs/               # 规范、版本矩阵、部署与开发说明、OpenAPI、阶段记录
├─ .claude/skills/     # AI Skill
├─ scripts/            # Skill 安装、镜像冒烟测试、CI 辅助脚本
└─ third_party/        # 第三方许可证全文
```

## 文档

| 想看什么 | 去哪里 |
| --- | --- |
| 用户文档（快速开始、配置、存储与规则、隐私与回收站） | [website/docs/guide](website/docs/guide)，站点的运行方法见 [website/README.md](website/README.md) |
| 部署 | [docs/deployment.md](docs/deployment.md) |
| 开发与配置项全表 | [docs/development.md](docs/development.md) |
| 接口契约 | [docs/openapi.yaml](docs/openapi.yaml) |
| 规范 | [docs/spec.md](docs/spec.md)（[在线版](https://claude.ai/code/artifact/a63820dd-642d-4916-bdd0-706cdfbd1fd8)，两边不一致时以仓库为准） |
| 各阶段实施记录 | [docs/planning](docs/planning) |

## 许可证

ImgNest 本身的许可证尚未确定。仓库中的第三方内容保留各自的许可证：界面基于的 Vben 为 MIT（`web-vben/vendor/vben/LICENSE`），31 个开源 Skill 为 MIT、Apache-2.0 或 CC-BY-SA-4.0，详见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) 和 `third_party/skills-licenses/`。
