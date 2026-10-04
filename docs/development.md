# ImgNest M1 开发与运行

M1 提供可运行的 Go 后端：SQLite/PostgreSQL、显式版本化迁移、管理员初始化、原生用户与 Token API。图片上传、libvips/WebP、回收站、蓝空 v1 和 Vue 页面在后续里程碑实现。

## 固定环境

使用 Docker Desktop 的 Linux 引擎。开发镜像 `golang:1.27.1-bookworm` 开启 cgo，测试数据库 `postgres:18.6-bookworm`。开发 compose 的 PostgreSQL 不向宿主公开端口，使用隔离容器网络中的免密码测试认证，不能当作生产部署配置。

源码挂载到 `/workspace`，Go 模块和构建缓存存命名卷。没有初始化时 `serve` 会拒绝启动，必须先执行 migrate。运行数据默认在 `data/imgnest.db`，不进 Git。

## SQLite 启动（PowerShell）

```powershell
docker compose -f deploy/compose.dev.yaml build dev
docker compose -f deploy/compose.dev.yaml run --rm dev go run ./cmd/imgnest migrate

$taskPassword = Read-Host '管理员密码（12–72 UTF-8 字节）' -AsSecureString
$taskPlainPassword = [System.Net.NetworkCredential]::new('', $taskPassword).Password
$taskPlainPassword | docker compose -f deploy/compose.dev.yaml run --rm -T dev go run ./cmd/imgnest init-admin --username admin --email admin@example.com
Remove-Variable taskPlainPassword, taskPassword

docker compose -f deploy/compose.dev.yaml run --rm --service-ports dev go run ./cmd/imgnest serve
```

将 username/email 改为实际管理员信息。密码通过 stdin 传入，不放在命令参数、文件或日志中；重复 init-admin 不会提升已有用户或覆盖管理员。注册默认关闭，管理设置的 HTTP/界面在后续阶段实现。

服务监听 [127.0.0.1:18080](http://127.0.0.1:18080)，健康检查为 [healthz](http://127.0.0.1:18080/healthz)。M1 根路径返回 JSON 404，前端尚未嵌入。Ctrl+C 触发关闭；编译的二进制同样支持 SIGINT/SIGTERM。

```powershell
Invoke-RestMethod http://127.0.0.1:18080/healthz
```

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

生产 PostgreSQL DSN 从部署配置/秘密环境注入，不打印或提交。先对选定数据库执行 migrate，再执行 init-admin 和 serve。迁移脚本被编译进程序，重复执行不会重复种子；脚本校验和改变、版本缺失或未知版本会报错，不能用 AutoMigrate 绕过。

SQLite 强制单连接、WAL（文件库）、foreign_keys 和 busy_timeout。PostgreSQL 使用连接池及事务锁；测试为每个用例建立独立临时 schema。

## API

精确契约在 [openapi.yaml](openapi.yaml)。原生响应固定 `code/message/data`，成功 code=0，失败 data=null；时间 UTC RFC3339。

| 路由 | 用途 |
| --- | --- |
| POST /api/auth/register | 开关开启后创建普通用户，不能提交角色/组 |
| POST /api/auth/login | 邮箱/密码换 24 小时 web Token |
| GET /api/auth/me | 当前账户与容量字节数 |
| POST /api/auth/logout | 吊销当前 Token |
| PATCH /api/auth/password | 验证旧密码、换新密码并吊销全部 Token |
| GET/POST /api/tokens | 无明文的列表 / 创建 api Token（可无到期时间） |
| DELETE /api/tokens/{id} | 只允许吊销本人 Token |

Bearer 明文只在 login/创建时返回。服务器仅存 secret 部分的 SHA-256；禁用、过期或撤销后不再有效。新密码为 12–72 字节；旧 bcrypt 凭证可短于 12 字节，但仍拒绝超过 72 字节的比较输入。登录/注册各每 IP 每分钟 3 次，单实例固定窗口；多实例需在后续设计共享限流。

签发会在事务中复查实际认证依据；改密、重置和撤销完成后，已经进入 HTTP 层但尚未完成的旧签发请求不能获得新 Token。并发改密通过旧哈希条件更新，避免覆盖已经完成的账户重置。

CLI 重置密码同样读取 stdin 并吊销全部凭证：

```powershell
$taskPassword = Read-Host '新密码' -AsSecureString
$taskPlainPassword = [System.Net.NetworkCredential]::new('', $taskPassword).Password
$taskPlainPassword | docker compose -f deploy/compose.dev.yaml run --rm -T dev go run ./cmd/imgnest reset-password --email admin@example.com
Remove-Variable taskPlainPassword, taskPassword
```

## 复现验收

```powershell
docker compose -f deploy/compose.dev.yaml up -d --wait postgres
docker compose -f deploy/compose.dev.yaml run --rm dev go test -mod=readonly -count=1 ./...
docker compose -f deploy/compose.dev.yaml run --rm dev go test -mod=readonly -race -count=1 ./...
docker compose -f deploy/compose.dev.yaml run --rm dev go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run ./...
docker compose -f deploy/compose.dev.yaml run --rm dev go mod verify
docker compose -f deploy/compose.dev.yaml run --rm dev go build -trimpath -o bin/imgnest ./cmd/imgnest
```

compose 自动把 `IMGNEST_TEST_POSTGRES_DSN` 指向测试服务。两库验收必须先启动 PostgreSQL，不能以未设置 DSN 导致的 skip 代替通过；不用生产数据库做测试。bcrypt race 测试较慢，等待实际退出状态。

`TestActualProcessSmoke` 编译实际二进制，启动两个 driver 的进程，验证健康检查、登录、重启后会话保留、API Token 创建/吊销和退出；临时数据库/schema 与进程由测试清理。HTTP 协议测试另覆盖未知字段、跨用户、伪造代理、输入上限、错误外壳和 panic 日志脱敏。

```powershell
docker compose -f deploy/compose.dev.yaml run --rm dev go test -mod=readonly ./internal/cli -run TestActualProcessSmoke -count=1 -v
docker compose -f deploy/compose.dev.yaml down
```

down 只停止本项目容器与网络，保留数据和缓存卷。执行记录见 [M1 progress](planning/m1-progress.md)。本阶段不发布 v1.0.0、不修改旧蓝空数据。
