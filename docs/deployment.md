# 部署

对外只发布 Docker 镜像：`ghcr.io/biliblihuorong/imgnest`（linux/amd64、linux/arm64）。SQLite 和 PostgreSQL 共用同一个镜像，数据库由环境变量在运行时选择，没有按数据库区分的镜像变体。不发布预编译二进制；需要二进制部署时用 `make release` 自行构建，运行机器须装有同版本 libvips 8.18.x，步骤见网站文档「部署」页（`website/docs/guide/deployment.md`）。

## 镜像 tag

| tag | 来源 | 用途 |
| --- | --- | --- |
| `1.2.3`、`1.2`、`latest` | `VERSION` 改为 `1.2.3` 后合并到 `main`（或手动推送 `v1.2.3` tag） | 正式版本，生产使用 |
| `edge` | `main` 每次推送 | 最新主线，可能不稳定 |
| `sha-<短提交>` | 每次发布 | 精确回滚 |

发布流程见 `.github/workflows/release.yml`：两个架构各自在原生 runner 上构建，通过 `scripts/smoke-image.sh` 冒烟测试后才推送，最后合并为一个多架构 manifest。

### 发布新版本

版本号写在仓库根目录的 `VERSION`。发版时在 PR 里执行 `python3 scripts/bump-version.py 1.2.3`，它同时改 `VERSION` 和 README、网站文档里写死的版本号。PR 合并到 `main` 后，`release.yml` 发现 `v1.2.3` 还没有 tag，就在推送 `edge` 的同一次运行里额外推送 `1.2.3`、`1.2`、`latest`，镜像推送成功后再用 `GITHUB_TOKEN` 创建 `v1.2.3` tag 和 GitHub Release（说明由 GitHub 按合并的 PR 自动生成）。不需要手动建 Release。

- 没改 `VERSION` 的合并只更新 `edge`。
- 带 `-` 的版本（如 `1.0.0-rc.1`）标为预发布，不更新 `latest`。
- 用 `GITHUB_TOKEN` 建的 tag 不会再触发其他 workflow，所以镜像在同一次运行里构建，而不是靠 tag 触发。
- 创建 Release 失败时，下一次推送 `main`（或手动运行 workflow）会重试。
- CI 的 `scripts/test-ci-config.py` 会检查文档里写的版本与 `VERSION` 一致。

## 启动

SQLite（单容器，个人自用）：

```bash
docker compose -f deploy/compose.sqlite.yaml up -d
```

PostgreSQL（多用户）。`POSTGRES_PASSWORD` 必须先写入环境变量或未跟踪的 `.env`：

```bash
docker compose -f deploy/compose.postgres.yaml up -d
```

容器入口在 `serve` 之前自动执行 `imgnest migrate`（版本化迁移，可重复执行）。其余子命令原样透传。

## 首次初始化

以 SQLite 为例，PostgreSQL 把文件名换成 `compose.postgres.yaml`。密码从标准输入读取，长度 12–72：

```bash
docker compose -f deploy/compose.sqlite.yaml exec -T imgnest imgnest init-admin --username admin --email admin@example.com < password.txt
```

创建本机存储和默认上传规则，`--base-url` 填对外访问的源：

```bash
docker compose -f deploy/compose.sqlite.yaml exec imgnest imgnest init-local --base-url https://img.example.com
```

## 配置

全部通过 `IMGNEST_*` 环境变量注入，完整列表见 [开发说明](development.md)。compose 文件已接好的变量：

| 变量 | 说明 |
| --- | --- |
| `IMGNEST_TAG` | 镜像 tag，默认 `latest` |
| `IMGNEST_PORT` | 宿主机端口，默认 8080 |
| `POSTGRES_PASSWORD` | 仅 PostgreSQL，必填 |
| `IMGNEST_SECURITY_MASTER_KEY` | 仅 S3 存储需要：32 字节随机数的 base64。丢失后已加密的存储凭据无法解密，需私下备份 |
| `IMGNEST_SERVER_TRUSTED_PROXIES` | 反向代理地址，逗号分隔 |
| `IMGNEST_IMAGES_VARIANT_CORS_ORIGINS` | 允许在浏览器里调用 `/api/variants` 的来源（如博客），逗号分隔 |
| `IMGNEST_LOG_LEVEL` | 日志级别：`debug` / `info`（默认）/ `warn` / `error` |

## 日志

- **运行日志**：JSON 一行一条，写到 stdout，用 `docker compose -f deploy/compose.sqlite.yaml logs -f imgnest` 查看。compose 已配置 Docker 轮转（单文件 20 MB，保留 5 个）。
  - 每个请求一条 `http request`：`request_id`、`method`、`route`（路由模板，不含真实路径和查询参数）、`status`、`duration_ms`、`client_ip`、`bytes`，已登录时有 `user_id`。5xx 记为 `ERROR` 并带 `error`（底层原因）和 `stack`（处理该错误的调用栈）。
  - 请求 ID 沿用反向代理传来的合法 `X-Request-ID`，否则自动生成，并在响应头里返回，可据此把用户报错和日志对上。请求期间其他日志也带同一个 `request_id`。
  - panic 记为 `request panic` / `… panicked`，带 `stack`；`panic` 字段只写运行时错误的消息（如空指针、越界），其他 panic 值只写类型，避免把请求数据写进日志。
  - 慢 SQL：超过 `log.slow_query`（默认 500ms，`IMGNEST_LOG_SLOW_QUERY`）记为 `slow database query`，SQL 里的字符串值被替换成 `'?'`。
  - 写日志前会遮盖 URL 里的账号密码、`password=` / `token=` 一类键值和 Bearer 令牌。
- **操作日志（审计）**：所有 `/api/` 下的 POST / PUT / PATCH / DELETE 请求（上传、删除、清空回收站、改设置、管理用户、登录等，含失败的）写入数据库 `audit_logs` 表，后台「操作日志」页面可按用户、只看失败筛选。只记账号、IP、路由、目标 ID、状态码和请求 ID，不记请求体。保留 `log.audit_days` 天（默认 90，`IMGNEST_LOG_AUDIT_DAYS`）。

## 数据与运行约束

- 数据卷 `/app/data`：SQLite 数据库、本机图片对象、缩略图缓存。PostgreSQL 另有 `postgres-data` 卷。
- 容器以非 root 用户 `imgnest`（uid 10001）运行。使用宿主机目录挂载时需先 `chown 10001:10001`。
- 单实例运行，不要对同一份数据启动多个副本。
- 健康检查为 `GET /healthz`，镜像已内置 `HEALTHCHECK`。
- 运行参数沿用规范 3.1：`LD_PRELOAD` jemalloc、`MALLOC_ARENA_MAX=2`。

## S3 / B2 存储的桶权限

图片直链由存储的 `base_url` 直接指向桶，所以桶通常是公开读的。删除图片时，对象会先复制到同一个桶的 `_trash/` 前缀下，保留期（默认 7 天）结束后才物理删除。**必须让 `_trash/` 前缀拒绝匿名读取**（存储设了路径前缀时是 `{路径前缀}/_trash/`），否则已删除的图片在保留期内仍能通过 `{base_url}/_trash/{原路径}` 打开：

- 公开桶（S3、R2、COS）：桶策略里对 `_trash/*` 加一条拒绝匿名 `GetObject` 的规则，或者只对图片所在前缀授予公开读。
- B2：B2 的公开桶不能按前缀限制，建议把桶设为私有，再通过 CDN（例如 Cloudflare）回源并在 CDN 上拒绝 `/_trash/` 路径。

回收站副本写入时带 `Cache-Control: private, no-store`，不会被 CDN 缓存；恢复后的对象重新使用长期缓存头。

已经被浏览器或 CDN 缓存的直链不会因为删除而失效（直链使用一年期 `immutable` 缓存）。需要立即下线时，请在 CDN 上手动清除对应 URL 的缓存。

## 自行构建

```bash
docker build -f deploy/Dockerfile -t imgnest:local .
```
