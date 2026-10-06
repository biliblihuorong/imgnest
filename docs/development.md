# ImgNest 开发与运行（M1–M5 / Vben 双前端）

当前提供配置/鉴权、图片核心、蓝空 v1 兼容与管理后台：SQLite/PostgreSQL、版本化迁移、本机/S3、同步 libvips/WebP、双缩略图、完整本地 EXIF、无损脱敏、可恢复回收站，嵌入二进制的 Vue 界面（登录/注册、上传、我的图片与回收站、相册、画廊、Token/账户），`/api/v1/*` 蓝空兼容（PicGo 可直连），以及 Vue 管理后台（用户/组/存储/规则/站点设置/全站图片）。M5 已提供相册、画廊及蓝空迁移能力。新版 Vben 界面独立位于 `web-vben/`，旧版 `web/` 保留；构建选择见下文。

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

服务监听 [127.0.0.1:18080](http://127.0.0.1:18080)，健康检查为 [healthz](http://127.0.0.1:18080/healthz)。根路径返回嵌入的 Vue 前端（`go:embed web/dist`）：未匹配的路径回退 `index.html`（no-store），`/assets/**` 带 immutable 缓存，`/api`、`/i`、`/t`、`/healthz` 前缀的未知路径保持 JSON 404。init-local 创建存储、默认规则并绑定默认组，实际本机访问前缀为 /i/{storage_id}。Ctrl+C 触发关闭；二进制同样支持 SIGINT/SIGTERM。单实例运行，监听前恢复遗留图片操作；恢复失败保留记录并拒绝就绪。

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

## 前端开发（M3）

前端在 `web/`：Vue 3.5 + Vite 8（Rolldown）+ TypeScript 6 + Naive UI + Pinia。Node 24.21.0 与 pnpm 12.9.1 已装入 dev 镜像，前端命令全部在容器内执行（宿主 Node 22 仅作手工便利，不作验收依据）；pnpm store 固定到 `pnpm-store` 卷（`web/pnpm-workspace.yaml` 的 storeDir）。

```powershell
make fe-install   # 校验冻结源码 + cd web && pnpm install --frozen-lockfile
make fe-build     # legacy 别名；--frozen-lockfile + vite build，不生成旧 API 类型
make fe-test      # vitest run（jsdom 组件测试）
make fe-lint      # vue-tsc --noEmit（双 tsconfig）+ eslint
make release      # release-legacy 别名；输出 bin/imgnest-legacy
```

- 请求只写在 `web/src/api/`：`client.ts` 解 `{code,message,data}` 外壳并统一错误（业务码 20001 清会话跳登录，20002 凭证内容错误就地展示）；`schema.d.ts` 由 `pnpm gen:api` 从 `docs/openapi.yaml` 生成，类型在 `api/types.ts` 派生，不手改。
- 全局状态只有 `stores/auth`（token + UserView，token 存 `localStorage["imgnest.token"]`）与 `stores/site`；列表数据留在页面。
- 路由守卫做本地 token 检查（无 token 深链跳 `/login?redirect=…`），真实鉴权由后端保证；Naive UI 组件显式 import，主题跟随系统。
- 上传通道用 XHR（fetch 无上传进度），与 fetch 通道共用 `client.notifyUnauthorized`，401 语义一致。
- `web/dist/.gitkeep` 保持无构建时 `go:embed` 可编译；`pnpm build` 会清空 dist，`make fe-build` 已在构建后恢复该占位文件。
- 已知限制：`vite.config.ts` 的 `@` 别名用 `import.meta.url` 解析，容器内正确；Windows 宿主直跑 `pnpm dev` 时别名可能失准（宿主不作验收环境）。Playwright E2E 延后到 M4 联调。

## 双前端独立构建（M5 / Vben 迁移）

`web/` 固定为实际 M5（`d19323d2714f9d050ffcde8221ef2738e3e98e53`）的旧前端；`web-vben/` 拥有独立源码、package、workspace、锁文件、node_modules 和 dist。选择发生在 Go 编译期，和登录用户的角色无关：默认 legacy；`-tags vben` 只导入新版 embed 包。两套资源不会一起链接到同一服务二进制，`web/embed.go` 保持 M5 原样。

```powershell
make fe-build-legacy    # 只安装 web 的冻结锁文件并构建 web/dist
make fe-build-vben      # 只安装 web-vben 的冻结锁文件并构建 web-vben/dist
make release-legacy    # 上述旧版前端 + bin/imgnest-legacy
make release-vben      # 上述新版前端 + -tags vben + bin/imgnest-vben
make release           # 默认仍为 legacy，输出 bin/imgnest-legacy
make build             # 仅编译默认 legacy 服务，沿用 bin/imgnest；不重建前端
make check-legacy-source
make check-frontend-selection
```

Make 命令继续使用已固定的 dev 容器；也可在具备相同版本的隔离本机环境中进入各自目录执行 `pnpm install --frozen-lockfile` 和 `pnpm build`。旧前端的安装/构建不依赖 Vben 包或新目录；两边不得互用 node_modules 或 dist。首次本机构建 legacy 时，按原 `web/pnpm-workspace.yaml` 使用可写 store，必要时仅通过 pnpm 命令行传入 `--store-dir`，不能为此改动旧配置。

普通构建不执行任何 `gen:api`。需要更新新版类型时单独执行 `make fe-gen-api-vben` 并审查差异；共享 OpenAPI 改动不能覆盖 `web/src/api/schema.d.ts`。旧版类型兼容性检查应输出到临时文件，再只读比较。

`docs/planning/legacy-m5-source.sha256` 固定旧版 102 个受保护源文件的 SHA-256，包含旧 embed、package、lock、workspace 与配置，排除 dist。`node scripts/check-legacy-source.mjs` 同时拒绝新增、缺失和内容变化；两种前端构建前后都会执行。node_modules、dist、coverage 是生成目录，不在源文件基线内。

`sh scripts/check-frontend-selection.sh` 使用真正的 Go `list -deps` 检查两种服务依赖图互斥，并分别编译/执行 selector 的文件系统测试。它不链接完整服务，因此不依赖 libvips；不能代替两套 release、完整后端回归或真实路由联调。完整服务仍需要版本矩阵中的 Go、libvips 与 C 库。辅助检查：`node --test scripts/check-legacy-source.test.mjs`；`python3 scripts/test-frontend-builds.py`（仅 Make 展开/路由检查，不执行 Docker 或编译）。

仅有 dist/.gitkeep 的干净检出允许 Go embed 包编译，但不是可发布前端；release 目标必须先产生真实 dist。切换二进制本身不会改变后端 API、数据库或安全配置；验证码启用后的旧版兼容约束仍按已批准方案处理，不能借切换前端绕过策略。

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
