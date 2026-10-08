# ImgNest 开发与运行（M1–M5）

当前提供配置/鉴权、图片核心、蓝空 v1 兼容与管理后台：SQLite/PostgreSQL、版本化迁移、本机/S3、同步 libvips/WebP、双缩略图、完整本地 EXIF、无损脱敏、可恢复回收站，嵌入二进制的 Vue 界面（登录/注册、上传、我的图片与回收站、相册、画廊、Token/账户），`/api/v1/*` 蓝空兼容（PicGo 可直连），以及 Vue 管理后台（用户/组/存储/规则/站点设置/全站图片）。M5 已提供相册与画廊；蓝空迁移工具 `import-lsky` 按规范延后，尚未提供。前端只有 `web-vben/`，构建见下文。

## 固定环境

使用 Docker Desktop 的 Linux 引擎。开发镜像以 imagor-base:vips8.18.6-r14-dev 为基础，复制 Go1.27.1 工具链，重编同版本 libvips 启用 BMP/Magick，并使用 jemalloc。源码 tarball 有固定 SHA256。当前验收为 Linux amd64。测试数据库 postgres:18.6-bookworm 与固定源码 MinIO 都不向宿主公开端口，仅在隔离容器网络使用测试认证。

源码挂载到 `/workspace`，Go 模块和构建缓存存命名卷。没有初始化时 `serve` 会拒绝启动，必须先执行 migrate。运行数据默认在 `data/imgnest.db`，不进 Git。

## SQLite 启动（PowerShell）

```powershell
docker compose -f deploy/compose.dev.yaml build dev
docker compose -f deploy/compose.dev.yaml run --rm dev go run ./cmd/imgnest migrate

$taskPassword = Read-Host '管理员密码（12–72 UTF-8 字节）' -AsSecureString
$taskPlainPassword = [System.Net.NetworkCredential]::new('', $taskPassword).Password
$taskPlainPassword | docker compose -f deploy/compose.dev.yaml run --rm -T dev go run ./cmd/imgnest init-admin --username admin --email admin@example.com
Remove-Variable taskPlainPassword, taskPassword

docker compose -f deploy/compose.dev.yaml run --rm dev go run ./cmd/imgnest init-local --base-url http://localhost:18080

docker compose -f deploy/compose.dev.yaml run --rm --service-ports dev go run ./cmd/imgnest serve
```

将 username/email 改为实际管理员信息。密码通过 stdin 传入，不放在命令参数、文件或日志中；重复 init-admin 不会提升已有用户或覆盖管理员。注册默认关闭，可由管理员在站点设置中控制；新版登录页不提供注册入口，直接注册路由仍遵循后端开关。

服务监听 [127.0.0.1:18080](http://127.0.0.1:18080)，健康检查为 [healthz](http://127.0.0.1:18080/healthz)。根路径返回嵌入的 Vue 前端（`go:embed web-vben/dist`）：未匹配的路径回退 `index.html`（no-store），`/assets/**` 带 immutable 缓存，`/api`、`/i`、`/t`、`/healthz` 前缀的未知路径保持 JSON 404。init-local 创建存储、默认规则并绑定默认组，实际本机访问前缀为 /i/{storage_id}。Ctrl+C 触发关闭；二进制同样支持 SIGINT/SIGTERM。单实例运行，监听前恢复遗留图片操作；恢复失败保留记录并拒绝就绪。

```powershell
Invoke-RestMethod http://127.0.0.1:18080/healthz
```

## 蓝空 v1 与 PicGo 联调

`/api/v1/*` 与蓝空 v1 字段级兼容（契约见项目 skill `lsky-api-compat`）：业务失败为 HTTP 200 + `status:false`；仅 401/403(API 禁用)/429 使用语义状态码；size/capacity 为 KB 浮点；全路由不返回 EXIF。`POST /api/v1/upload` 在开启游客上传后允许匿名（按游客组规则与每 IP 限流）；携带但无效的 Token 一律 401，不降级。

PicGo（picgo-plugin-lankong）或 uPic 手工联调步骤：

1. 先取 Token：`curl -X POST http://127.0.0.1:18080/api/v1/tokens --data-urlencode 'email=…' --data-urlencode 'password=…'`，`data.token` 即 Bearer 值（也可以在网页 Token 页创建）。
2. 插件配置：API 地址 `http://<host>:<port>/api/v1/upload`，鉴权选 Bearer 并粘贴 Token，域名即为返回的 `links.url`。
3. 上传一张 PNG，确认返回 URL 可直接打开、`links.url` 与策略 `link_prefer` 一致（默认 WebP）、PicGo 相册能看到缩略图（`thumbnail_url`）。

管理后台：admin 登录后访问 `/admin`（用户/用户组/存储/规则/站点设置/全站图片）。S3 存储凭据 AES-GCM 加密落库，创建后任何接口不回显；「测试连接」执行不覆盖写、复制、清理三项真实探测。站点设置可改站点名（M3 的 `/api/site` 立即生效）、注册/游客上传/画廊开关、回收站天数与 API 开关（`api_enabled=false` 时全部 /api/v1 返回 403）。

## 配置与 PostgreSQL

复制 `deploy/config.example.yaml` 到未跟踪的 `config.yaml`，用 `--config config.yaml` 显式加载。配置顺序为默认值、YAML、环境变量；错误内容不会回显凭证或整个配置。

| 配置 | 环境变量 | 默认 |
| --- | --- | --- |
| server.addr | IMGNEST_SERVER_ADDR | :8080 |
| server.read_header_timeout | IMGNEST_SERVER_READ_HEADER_TIMEOUT | 5s |
| server.shutdown_timeout | IMGNEST_SERVER_SHUTDOWN_TIMEOUT | 10s |
| server.trusted_proxies | IMGNEST_SERVER_TRUSTED_PROXIES | 空；忽略伪造转发头 |
| database.driver | IMGNEST_DATABASE_DRIVER | sqlite |
| database.dsn | IMGNEST_DATABASE_DSN | data/imgnest.db；PostgreSQL 必须显式提供 |
| database.max_open/max_idle | IMGNEST_DATABASE_MAX_OPEN/MAX_IDLE | SQLite 1/1，PG 25/10 |
| database.max_lifetime | IMGNEST_DATABASE_MAX_LIFETIME | SQLite 0，PG 5m |
| database.busy_timeout | IMGNEST_DATABASE_BUSY_TIMEOUT | 5s |
| server.max_upload_mb | IMGNEST_SERVER_MAX_UPLOAD_MB | 20；1–20，单个源文件 |
| server.max_request_mb | IMGNEST_SERVER_MAX_REQUEST_MB | 64；含 multipart 尾部，最多256 |
| server.upload_concurrency | IMGNEST_SERVER_UPLOAD_CONCURRENCY | 2；有界请求内存 |
| server.processing_timeout | IMGNEST_SERVER_PROCESSING_TIMEOUT | 5m；最多30m |
| server.max_pixels | IMGNEST_SERVER_MAX_PIXELS | 100000000；含实际加载动画帧 |
| images.thumb_cache | IMGNEST_IMAGES_THUMB_CACHE | data/thumbs；普通WebP可再生成 |
| security.master_key | IMGNEST_SECURITY_MASTER_KEY | 本机可空；S3需base64编码32bytes |

生产 PostgreSQL DSN 从部署配置/秘密环境注入，不打印或提交。先对选定数据库执行 migrate，再执行 init-admin 和 serve。迁移脚本被编译进程序，重复执行不会重复种子；脚本校验和改变、版本缺失或未知版本会报错，不能用 AutoMigrate 绕过。

SQLite 强制单连接、WAL（文件库）、foreign_keys 和 busy_timeout。PostgreSQL 使用连接池及事务锁；测试为每个用例建立独立临时 schema。

## API

精确契约在 [openapi.yaml](openapi.yaml)。原生响应固定 `code/message/data`，成功 code=0，普通失败 data=null；统一搜索错误在 data.diagnostics 返回结构化诊断。时间 UTC RFC3339。

| 路由 | 用途 |
| --- | --- |
| POST /api/auth/register | 开关开启后创建普通用户，不能提交角色/组 |
| POST /api/auth/login | 邮箱/密码换 24 小时 web Token |
| GET /api/auth/me | 当前账户与容量字节数 |
| POST /api/auth/logout | 吊销当前 Token |
| PATCH /api/auth/password | 验证旧密码、换新密码并吊销全部 Token |
| GET/POST /api/tokens | 无明文的列表 / 创建 api Token（可无到期时间） |
| DELETE /api/tokens/{id} | 只允许吊销本人 Token |
| GET /api/site | 公开站点摘要：site_name 与 register_enabled |
| GET /api/policies | 当前用户组绑定的启用规则（含存储启用），上传页下拉用 |
| POST /api/upload | file/files[] multipart，同步单201/批量207；默认private |
| GET /api/images；GET /api/images/{id} | 本人分页/详情，无EXIF |
| GET /api/images/{id}/exif | 本人/admin的原始完整EXIF/GPS/raw |
| PATCH /api/images/{id} | is_public；持有直链仍可访问private |
| DELETE /api/images/{id} | 移入回收站；成功时原Key404 |
| POST /api/images/batch | action=delete/permission，ids数组，逐项207 |
| GET /api/trash；POST /api/trash/restore、purge | 鉴权操作，输入ids数组 |
| /i/{storage_id}/{path} | 仅active声明的本机对象，匿名直链 |
| /t/{key}.webp | private/trash仅本人/admin；缺失懒回填 |

Bearer 明文只在 login/创建时返回。服务器仅存 secret 部分的 SHA-256；禁用、过期或撤销后不再有效。新密码为 12–72 字节；旧 bcrypt 凭证可短于 12 字节，但仍拒绝超过 72 字节的比较输入。登录/注册各每 IP 每分钟 3 次，单实例固定窗口；多实例需在后续设计共享限流。

签发会在事务中复查实际认证依据；改密、重置和撤销完成后，已经进入 HTTP 层但尚未完成的旧签发请求不能获得新 Token。并发改密通过旧哈希条件更新，避免覆盖已经完成的账户重置。

CLI 重置密码同样读取 stdin 并吊销全部凭证：

```powershell
$taskPassword = Read-Host '新密码' -AsSecureString
$taskPlainPassword = [System.Net.NetworkCredential]::new('', $taskPassword).Password
$taskPlainPassword | docker compose -f deploy/compose.dev.yaml run --rm -T dev go run ./cmd/imgnest reset-password --email admin@example.com
Remove-Variable taskPlainPassword, taskPassword
```

## 前端开发

前端只有 `web-vben/`：基于 Vben 5.8.0 源码工作区的 Vue 3.5 + Vite 8（Rolldown）+ TypeScript 6 + Naive UI + Pinia 应用，构建后由 `web-vben/embed.go` 嵌入服务二进制。M5 时期的旧前端 `web/` 及其 `vben` 构建标签、冻结源码校验已于 2026-10-07 移除。Node 24.21.0 与 pnpm 12.9.1 已装入 dev 镜像，前端命令全部在容器内执行（宿主 Node 22 仅作手工便利，不作验收依据）。

```powershell
make fe-install   # cd web-vben && pnpm install --frozen-lockfile
make fe-build     # --frozen-lockfile + vite build，构建后恢复 dist/.gitkeep
make fe-test      # vitest run
make fe-lint      # vue-tsc --noEmit（双 tsconfig）+ eslint
make fe-gen-api   # 从 docs/openapi.yaml 重新生成 src/api/schema.d.ts
make release      # fe-build + 输出 bin/imgnest
make build        # 仅编译服务，不重建前端
```

- 请求只写在 `web-vben/src/api/`；`schema.d.ts` 由 `pnpm gen:api` 生成，不手改。普通构建不执行 `gen:api`，需要更新类型时单独执行 `make fe-gen-api` 并审查差异。
- `web-vben/dist/.gitkeep` 保持无构建时 `go:embed` 可编译，但这样的二进制不含可用前端；发布必须先产生真实 dist。生产镜像在 `deploy/Dockerfile` 内完成前端构建。
- `internal/cli` 的 `TestFrontendDistFS` 校验嵌入的文件系统与 `web-vben/dist` 逐字节一致。

## 复现验收

```powershell
docker compose -f deploy/compose.dev.yaml build dev minio
docker compose -f deploy/compose.dev.yaml up -d --wait postgres minio
docker compose -f deploy/compose.dev.yaml run --rm dev go test -mod=readonly -count=1 ./...
docker compose -f deploy/compose.dev.yaml run --rm dev go test -mod=readonly -race -count=1 ./...
docker compose -f deploy/compose.dev.yaml run --rm dev go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run ./...
docker compose -f deploy/compose.dev.yaml run --rm dev go mod verify
make fe-test
make fe-lint
make release
```

compose 自动注入双库/MinIO测试环境。必须启动相应测试服务，不能以未配置导致的 skip 代替通过；不用生产数据或云账户做测试。bcrypt race 测试较慢，等待实际退出状态。

`TestActualProcessSmoke` 编译实际二进制，启动两库进程，验证健康检查、登录、真实curl上传→原/WebP/thumb直链→trash404→restore→purge、重启会话和Token吊销/退出。MinIO组合测试另验证加密配置、实际云字节计费与所有版本/markers清空。HTTP真组件测试包含GPS/XMP脱敏、原始压缩像素保留、私有直链与EXIF权限；竞态回归覆盖旧缓存回填和双清理路径复用。

```powershell
docker compose -f deploy/compose.dev.yaml run --rm dev go test -mod=readonly ./internal/cli -run TestActualProcessSmoke -count=1 -v
docker compose -f deploy/compose.dev.yaml down
```

down 停止本项目容器/网络，保留数据库和Go缓存卷；MinIO测试数据在tmpfs，容器删除后消失。执行记录见 [M2 progress](planning/m2-progress.md)。本阶段不发布 v1.0.0，不操作旧蓝空数据。

## 上传与 S3 初始化

上传接受 file/files[]、policy_id、album_id、is_public，不信任文件名和 Content-Type。单文件返回 ImageView，批量为207逐项结果。脱敏失败为50003/422；对象写入或最终事务失败按归属补偿，失败保留路径和清理记录。

本机存储根的 .jpg/.webp 是内部单文件封装，用于原子保存归属和未重编码内容。只通过程序/Driver.Open访问，备份保留整个根；不要把物理目录当普通静态图片目录挂出。data/thumbs/<storage_id>/<path>_thumbs.webp 仍是普通WebP缓存，可删后重建。

S3主密钥从 IMGNEST_SECURITY_MASTER_KEY 或未跟踪部署配置注入（32随机bytes的base64），和私有备份一起保管。把配置存入未跟踪私有文件，替换占位后通过 stdin 输入；命令仅输出无密钥 StorageView：

```json
{"name":"cloud","driver":"s3","base_url":"https://images.example.com","config":{"endpoint":"https://s3.example.com","region":"us-east-1","bucket":"your-bucket","access_key_id":"YOUR_ACCESS_KEY","secret_access_key":"YOUR_SECRET","use_path_style":true}}
```

```powershell
Get-Content -Raw .\private-storage.json | docker compose -f deploy/compose.dev.yaml run --rm -T dev go run ./cmd/imgnest init-storage
docker compose -f deploy/compose.dev.yaml run --rm dev go run ./cmd/imgnest init-policy --storage-id 2 --name cloud
```

storage-id用实际输出ID；init-policy --stdin 可覆盖path_tpl/name_tpl/webp_mode/scrub_mode等默认字段。连接测试验证不覆盖写入、条件服务端复制和清理；不支持的兼容端明确拒绝。真实B2/COS/R2账户未在本轮配置，需单独联调。

used_bytes 是唯一cloud Key实际字节，含云thumb，本地缓存不计费。删除扣费一次并保留7天；恢复重新预约容量/检查认证，激活后清除trash。彻底删除先确认全部版本归属，再逐VersionID清理；外部历史会阻止任务，避免误删。EXIF raw仅本人/admin可读，不透明classicTIFF的显式full-source-fallback可能含至多20MiB源文件；BigTIFF/不支持元数据布局拒绝，见规范8.2。
