# 部署

对外只发布 Docker 镜像：`ghcr.io/biliblihuorong/imgnest`（linux/amd64、linux/arm64）。SQLite 和 PostgreSQL 共用同一个镜像，数据库由环境变量在运行时选择，没有按数据库区分的镜像变体。

## 镜像 tag

| tag | 来源 | 用途 |
| --- | --- | --- |
| `1.2.3`、`1.2`、`latest` | 推送 `v1.2.3` tag | 正式版本，生产使用 |
| `edge` | `main` 每次推送 | 最新主线，可能不稳定 |
| `sha-<短提交>` | 每次发布 | 精确回滚 |

发布流程见 `.github/workflows/release.yml`：两个架构各自在原生 runner 上构建，通过 `scripts/smoke-image.sh` 冒烟测试后才推送，最后合并为一个多架构 manifest。

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

## 数据与运行约束

- 数据卷 `/app/data`：SQLite 数据库、本机图片对象、缩略图缓存。PostgreSQL 另有 `postgres-data` 卷。
- 容器以非 root 用户 `imgnest`（uid 10001）运行。使用宿主机目录挂载时需先 `chown 10001:10001`。
- 单实例运行，不要对同一份数据启动多个副本。
- 健康检查为 `GET /healthz`，镜像已内置 `HEALTHCHECK`。
- 运行参数沿用规范 3.1：`LD_PRELOAD` jemalloc、`MALLOC_ARENA_MAX=2`。

## S3 / B2 存储的桶权限

图片直链由存储的 `base_url` 直接指向桶，所以桶通常是公开读的。删除图片时，对象会先复制到同一个桶的 `_trash/` 前缀下，保留期（默认 7 天）结束后才物理删除。**必须让 `_trash/` 前缀拒绝匿名读取**，否则已删除的图片在保留期内仍能通过 `{base_url}/_trash/{原路径}` 打开：

- 公开桶（S3、R2、COS）：桶策略里对 `_trash/*` 加一条拒绝匿名 `GetObject` 的规则，或者只对图片所在前缀授予公开读。
- B2：B2 的公开桶不能按前缀限制，建议把桶设为私有，再通过 CDN（例如 Cloudflare）回源并在 CDN 上拒绝 `/_trash/` 路径。

回收站副本写入时带 `Cache-Control: private, no-store`，不会被 CDN 缓存；恢复后的对象重新使用长期缓存头。

已经被浏览器或 CDN 缓存的直链不会因为删除而失效（直链使用一年期 `immutable` 缓存）。需要立即下线时，请在 CDN 上手动清除对应 URL 的缓存。

## 自行构建

```bash
docker build -f deploy/Dockerfile -t imgnest:local .
```
