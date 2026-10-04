# ImgNest M1 Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. 本起步包只附带 executing-plans，其他工作流及脚本须在执行前查找；未找到时报告限制，不伪造调用。执行方式由用户审阅计划后选择。

**Goal:** 在锁定的 Linux 工具链中交付支持 SQLite/PostgreSQL 的可运行后端：配置、版本化迁移、用户与 Token 鉴权、原生 API 和可复现验证。

**Architecture:** 沿用 spec 的 `http → service → repo`；service 定义消费的接口，入口手动注入 GORM repo 与时钟。迁移采用嵌入的版本化 SQL，以 GORM 底层 `database/sql` 执行，CLI 与 HTTP 共用 service。M1 不依赖 libvips，不创建未使用的存储、图片处理、前端或通用 CRUD 框架。

**Tech Stack:** Go 1.27.1、Gin 1.12.0、GORM 1.31.2、postgres driver 1.6.3、sqlite driver 1.6.0、koanf 2.3.7 及锁定 provider/parser、Cobra 1.10.2、x/crypto 0.57.0、testify 1.12.1；slog 与其他 Go 标准库。PostgreSQL 验证镜像 18.6。

**Spec:** [docs/spec.md](../../spec.md)、[docs/versions.md](../../versions.md)、[开工审查](../../planning/2026-10-04-readiness.md)。module 路径由用户指定为 `github.com/biliblihuorong/imgnest`。

**Status:** 待用户审阅；下列 checkbox 是实施工作，尚未执行。

## Global Constraints

- 一个仓库、一个 Go module；前端最终放 `web/`，本阶段不安装前端依赖。
- `go 1.27.0` + `toolchain go1.27.1`；只使用 versions.md 中的精确直接依赖，首次 tidy 后提交实际 go.sum。只引入本阶段使用的模块。
- 分层单向；service 不导入具体 repo、GORM、Gin 或存储实现；repo 负责 SQL/GORM；组合根位于 CLI。
- 新增 Go 对外函数首参为 `context.Context`；固定框架签名除外，内部继续传递 context。
- `%w` 包装技术错误，`errors.Is` 匹配业务哨兵；native 外壳 `{"code","message","data"}`；时间 RFC3339。
- PostgreSQL + SQLite 作为主测试目标；生产路径无 AutoMigrate；已发布迁移只能通过新版本补充。
- 明文 Token 只创建时返回；格式 `<id>|<40 random chars>`，SHA-256 只计算 `|` 后的随机串，常量时间比较；密码 bcrypt。
- 日志不得出现密码、Token、密钥、GPS、认证请求正文或带凭证的数据库 DSN；配置不能整体输出。
- 功能分支 `feat/m1-foundation`；Conventional Commits；不得在 main 上直接实现。
- 上传补偿、对象 Key、EXIF 与蓝空字段约束继续有效，但属于后续阶段，M1 不创建上传占位接口。

## 补充设计（本计划提议，尚未写入规范）

以下值不是计划书已经给出的事实，审阅本计划即审阅这些选择。采纳后先同步到 `spec.md` 的相应章节；涉及 Token 的项目 Skill 同步更新。

| 项目 | M1 的具体选择 |
| --- | --- |
| 配置优先级 | defaults < 显式 YAML < `IMGNEST_` 环境变量；`--config` 不存在或 YAML 无效时报错 |
| 配置默认 | `server.addr=:8080`、read_header_timeout=5 s、shutdown_timeout=10 s、trusted_proxies=[]；`database.driver=sqlite`；`database.dsn=data/imgnest.db`；SQLite WAL、foreign_keys=ON、busy_timeout=5000 ms、max_open=max_idle=1、max_lifetime=0 |
| PostgreSQL pool | max_open=25、max_idle=10、max_lifetime=5 min；通过配置覆盖 |
| 站点种子 | registration_enabled=false、guest_upload_enabled=false、gallery_enabled=false、trash_days=7；默认普通组容量 0 表示不限；组上传参数在 M2 使用前补齐 |
| 管理员初始化 | 显式 `imgnest init-admin --username ... --email ...`，密码从 stdin 读取；普通注册永远 role=user，不采用“第一个注册即管理员” |
| 用户验证 | username 3–64 runes，email TrimSpace + 小写且通过 `net/mail` 验证；密码 12–72 bytes；bcrypt 默认 cost=12 |
| Token | web kind 默认有效期 24 h；api kind 的过期时间可为空，否则须晚于当前时刻；abilities 在 M1 仅允许 `["*"]`；活动 Token 的用户必须仍 enabled |
| 凭证清理 | logout 仅吊销当前 Token；改密/reset-password 吊销该用户全部 Token；用户 A 不能查看/吊销 B 的 Token |
| 登录限流 | 登录与注册按真实客户端 IP 各 3 次/分钟；默认不信任转发代理，后续按配置显式允许 |
| 新增改密路由 | `PATCH /api/auth/password`，输入 `current_password/new_password`，成功吊销全部 Token，前端需重新登录 |
| native code | 10001 参数错误；20001 未鉴权；20002 凭证错误；20003 权限不足；30001 注册关闭；30002 用户已存在；30003 请求限流；50001 内部处理失败 |
| 时间与脱敏 | DB 存 UTC，HTTP 序列化 RFC3339；DTO 不出现 password_hash/token_hash；Token 列表不返 secret |
| 迁移分批 | 0001 只建 groups/users/tokens/settings 与 schema_migrations；images 等由 M2 新迁移创建 |
| 迁移执行 | CLI `migrate` 显式执行；serve 校验 schema 已到本二进制期望版本，版本缺失/未知即拒绝启动；不偷偷修改生产 schema |

## Review Focus

1. 并发注册同一 email 或 username：只一笔成功，无孤儿 Token；Task 4 的双数据库竞争测试覆盖。
2. 过期边界、用户禁用、撤销后旧凭证：一律拒绝；Task 5 的固定时钟和 repo 集成测试覆盖。
3. 迁移中途失败、重复执行、已执行脚本变动：事务回滚，不重复种子，校验和不符停止；Task 3 覆盖。
4. 跨用户 Token 操作、注册夹带 admin/group_id、伪造代理头限流：不能越权；Task 6 覆盖。
5. 取消请求、数据库连接错误与关闭服务时的日志：无敏感值，资源关闭；Tasks 2、7、8 覆盖。

## 文件与任务边界

| 文件/目录 | 职责 | 任务 |
| --- | --- | --- |
| go.mod、go.sum、.gitignore、.golangci.yml、Makefile | module、精确依赖与基础验证 | 1 / 8 |
| cmd/imgnest/main.go、internal/cli/root.go | 极薄入口、Cobra 与组合根 | 1 / 7 |
| deploy/Dockerfile.dev、deploy/compose.dev.yaml | 固定 Go 开发/test 环境、临时 PG 集成环境 | 1 / 3 |
| internal/config/config.go | 部署参数读取与验证 | 2 |
| internal/repo/database.go、user.go、token.go、settings.go | 数据库连接与 GORM 实现 | 3 / 4 / 5 |
| internal/model/{group,user,token,setting}.go | 不带 HTTP 输出职责的持久化模型 | 3 |
| internal/migrate/{migrate.go,sqlite/0001_auth.sql,postgres/0001_auth.sql} | 迁移清单、事务与版本记录 | 3 |
| internal/service/{errors.go,user.go,token.go,settings.go} | 接口、业务校验与鉴权 | 4 / 5 |
| internal/http/{router.go,response.go,middleware.go,native/auth.go,native/token.go} | 路由、DTO、参数、鉴权与错误映射 | 6 |
| internal/cli/{serve,migrate,admin}.go | 生命周期、管理员初始化与重置密码 | 7 |
| docs/openapi.yaml、docs/development.md | 可实现的 API 契约与复现操作 | 6 / 8 |
| 相应 `_test.go` 与 `internal/http/native/testdata/` | 同目录单测、双库集成与 native JSON | 各任务 |

构造器及服务方法都传 context。repo 的 public 方法与下面接口相同；生产 HTTP DTO 和持久化模型分开。时钟通过 `func() time.Time` 传入 Token service；不额外创建 clock 包。

## Task 1: 可复现的后端入口与验证环境

**Files:** 创建 go.mod/go.sum、cmd/imgnest/main.go、internal/cli/root.go/root_test.go、deploy/Dockerfile.dev、deploy/compose.dev.yaml、.gitignore、.golangci.yml、Makefile。

**Interfaces:** 产出 `cli.Execute(ctx context.Context, args []string, stdout, stderr io.Writer) error`；开发 compose 的 `dev` service 在 `/workspace` 执行 Go，`postgres` 为隔离测试数据库，不写宿主生产 data。

- [ ] 核对本地/远端所有 refs；若远端无提交，初始化 main、为起步包建立基线提交、设置 origin 后切 feature 分支；若有提交，先比对目录内容并保留历史。记录实际起点，不强推。
- [ ] 把本计划已采纳的补充设计同步到 spec.md 以及适用的项目 Skill，建立本次执行的进度文件 `docs/planning/m1-progress.md`，记录任务状态、测试结果与任何偏离；不调用缺失的上游脚本。提交文档基线，之后才实现业务。
- [ ] 创建 module，准确填写 Go 指令；开发镜像使用 `golang:1.27.1-bookworm`，保持 CGO_ENABLED=1，不装 libvips。只加入本任务用到的锁定 Cobra。配置持久化构建 cache、源码挂载及以 2.14.0 运行 lint 的方式。
- [ ] 先写 `TestExecuteHelp`：Execute `--help` 返回 nil，输出包含 `ImgNest`；`TestExecuteUnknownCommand`：未知命令返回 error。测试使用 bytes.Buffer，不启动进程。

```go
func TestExecuteHelp(t *testing.T) {
    var out, errOut bytes.Buffer
    if err := cli.Execute(t.Context(), []string{"--help"}, &out, &errOut); err != nil {
        t.Fatal(err)
    }
    if !strings.Contains(out.String(), "ImgNest") {
        t.Fatalf("help does not identify ImgNest: %q", out.String())
    }
}
```

- [ ] Run: `docker compose -f deploy/compose.dev.yaml run --rm dev go test ./internal/cli -run TestExecute -v`。Expected: RED，Execute 尚未定义，记录失败输出。
- [ ] 实现只含帮助的根命令和 main。之后同一命令 Expected: PASS；`go run ./cmd/imgnest --help` Expected: exit 0。tidy/verify 后锁定 go.sum，提交 `chore: bootstrap reproducible Go development environment`。

## Task 2: YAML 与环境变量配置

**Files:** 创建 internal/config/config.go/config_test.go、deploy/config.example.yaml；修改 go.mod/go.sum。

**Interfaces:** 消费 Task 1 的工具链。产出 `config.Load(ctx context.Context, path string, environ []string) (Config, error)`。Config 具有 Server、Database；Database 的 Driver/DSN 为 string、MaxOpen/MaxIdle 为 int、MaxLifetime/BusyTimeout 为 time.Duration；Server 的 Addr 为 string、TrustedProxies 为 []string、ReadHeaderTimeout/ShutdownTimeout 为 time.Duration。环境变量按已知配置 key 映射，例如 `IMGNEST_DATABASE_MAX_OPEN → database.max_open`，不将字段名内部所有下划线改成点。

- [ ] 先写 `TestLoadPrecedence`，临时 YAML 的 addr=:8090、环境 IMGNEST_SERVER_ADDR=:8091，断言 :8091；`TestLoadDefaults` 断言 sqlite/5000 ms/单连接；`TestLoadInvalidDriver`、`TestLoadMissingExplicitFile`、`TestLoadCancelledContext` 均断言 error。`TestConfigErrorsRedactSecrets` 输入包含测试 DSN 密码，断言 error 不含该密码。
- [ ] Run: `docker compose -f deploy/compose.dev.yaml run --rm dev go test ./internal/config -v`。Expected: RED，配置 API 尚未实现。
- [ ] 用锁定 koanf/confmap/file/env/yaml 实现上述 Load；未给 path 时只用默认与环境；非法数值、未知 driver、负 timeout 和 malformed YAML 拒绝。省略真实密钥样例，不打印 cfg 或 DSN。
- [ ] 同一测试命令 Expected: PASS；示例 YAML 不含秘密、值与本计划默认一致。记录实际安装与类型检查结果。
- [ ] `go mod tidy`、`go mod verify` 后提交 `feat: load validated deployment configuration`。

核心取消断言：

```go
func TestLoadCancelledContext(t *testing.T) {
    ctx, cancel := context.WithCancel(t.Context())
    cancel()
    _, err := config.Load(ctx, "", nil)
    if !errors.Is(err, context.Canceled) {
        t.Fatalf("got %v, want context.Canceled", err)
    }
}
```

## Task 3: 双数据库连接与不可变迁移

**Files:** 创建 internal/repo/database.go/database_test.go、internal/model 四个文件、internal/migrate/migrate.go/migrate_test.go 与两份 0001_auth.sql；修改 compose.dev.yaml、go.mod/go.sum。

**Interfaces:** 消费 config.Database。产出 `repo.Open(ctx context.Context, cfg config.Database) (*gorm.DB, error)`、`migrate.Up(ctx context.Context, db *sql.DB, driver string) error`、`migrate.Check(ctx context.Context, db *sql.DB, driver string) error`。调用方通过 `gormDB.DB()` 获取 sqlDB 并负责 Close。

模型按 spec §4 使用普通字段和 GORM tag：User(ID/GroupID uint64、Username/Email/PasswordHash/Role/Status string、UsedBytes int64)；Token(ID/UserID uint64、Name/TokenHash/Kind string、Abilities []string、LastUsedAt/ExpiresAt *time.Time)；Group 的 ID uint64、Name string、IsDefault/IsGuest bool、CapacityBytes/MaxFileBytes int64、AllowedExts []string、UploadPerMin int；Setting(Key string、Value json.RawMessage)。业务表均含 CreatedAt/UpdatedAt time.Time；User Status 值为 enabled/disabled，Role 为 admin/user。schema_migrations 记录 version、checksum、applied_at。JSON 的数据库表示在 repo 内转换，service 不依赖 GORM JSON 类型。

- [ ] 先写 `TestMigrateFreshDatabase`：0001 后四张业务表及 schema_migrations 存在；`TestMigrateTwice`：默认组与 settings 各一份；`TestMigrationFailureRollsBack`：使用包内测试迁移清单，注入错误 SQL，断言表/版本记录回滚；`TestMigrationChecksumMismatch`、`TestCheckPendingAndUnknownVersion`：错误必须被返回。
- [ ] 写 `TestSQLitePragmas`，在临时文件库检查 WAL/foreign_keys/busy_timeout 与单连接；每个 DB 测试都对 sqlite 执行一次、对显式提供 `IMGNEST_TEST_POSTGRES_DSN` 的 PostgreSQL 执行一次。PG runner 未提供 DSN 可 skip，但 Task 8 验收必须启用 PG，不能以 skip 宣称通过。
- [ ] Run: `docker compose -f deploy/compose.dev.yaml run --rm dev go test ./internal/migrate ./internal/repo -v`。Expected: RED，数据库入口或迁移尚未存在。
- [ ] 实现 GORM 连接的 context、pool、Ping、脱敏错误；embed 迁移、按序校验 checksum/版本、每版事务执行及种子。SQLite 文件父目录归 data；不对 in-memory DB 要求 WAL。PG 用 advisory lock 串行迁移；SQLite 用写锁防止同时迁移。用户状态与种子按本计划设定；email/username 唯一，users.group_id/tokens.user_id 外键，expires_at/last_used_at 可 NULL。default_policy_id 及其他 M2 列在下一版本加入。
- [ ] SQLite 与 PG 同一组测试 Expected: PASS。检查生产代码无 AutoMigrate；核对实际间接依赖再校正 versions.md 备注并在审查记录写明选择。提交 `feat: add versioned auth migrations for SQLite and PostgreSQL`。

`TestMigrateTwice` 对两个 driver 分别创建隔离库，在同一 sqlDB 上 Up 两次后的核心断言：

```go
var defaults int
err := sqlDB.QueryRowContext(t.Context(),
    "SELECT COUNT(*) FROM groups WHERE is_default = true").Scan(&defaults)
if err != nil || defaults != 1 {
    t.Fatalf("default groups=%d, err=%v; want exactly one", defaults, err)
}
```

## Task 4: 用户注册、密码与管理员初始化业务

**Files:** 创建/修改 internal/service/errors.go/user.go/user_test.go/settings.go、internal/repo/user.go/settings.go 及集成测试。

**Interfaces:**

- `UserRepository`：`CreateUser(ctx context.Context, user model.User) (model.User, error)`、`FindUserByEmail(ctx context.Context, email string) (model.User, error)`、`FindUserByID(ctx context.Context, id uint64) (model.User, error)`、`BootstrapAdmin(ctx context.Context, user model.User) (model.User, error)`、`UpdatePasswordAndRevokeTokens(ctx context.Context, userID uint64, hash string) error`。
- `SettingsRepository`：`RegistrationEnabled(ctx context.Context) (bool, error)`、`DefaultGroupID(ctx context.Context) (uint64, error)`。
- repo 构造器：`NewUserRepository(ctx context.Context, db *gorm.DB) (*UserRepository, error)`、`NewSettingsRepository(ctx context.Context, db *gorm.DB) (*SettingsRepository, error)`。这里的 concrete repo 类型与 service 的同名接口位于不同 package。
- `NewUserService(ctx context.Context, users UserRepository, settings SettingsRepository) (*UserService, error)`；方法 `Register(ctx context.Context, input RegisterInput) (UserView, error)`、`VerifyCredentials(ctx context.Context, email, password string) (UserView, error)`、`InitAdmin(ctx context.Context, input RegisterInput) (UserView, error)`、`ChangePassword(ctx context.Context, userID uint64, current, next string) error`、`ResetPassword(ctx context.Context, email, next string) error`。
- RegisterInput：Username/Email/Password strings。UserView：ID/GroupID uint64，Username/Email/Role/Status strings，UsedBytes int64，CreatedAt time.Time。哨兵定义在 errors.go：ErrInvalidInput、ErrNotFound、ErrRegistrationDisabled、ErrUserExists、ErrInvalidCredentials、ErrForbidden、ErrUnauthenticated。

- [ ] 先写 `TestRegisterDisabled`、`TestRegisterUsesDefaultGroupAndUserRole`、`TestPasswordByteLimits`（11/12/72/73 bytes）、`TestVerifyWrongCredentials`（不存在与密码错都是 ErrInvalidCredentials）、`TestInitAdminRequiresNoExistingAdmin`、`TestChangePasswordRevokesAllTokens`。
- [ ] Run: `docker compose -f deploy/compose.dev.yaml run --rm dev go test ./internal/service -v`。Expected: RED。
- [ ] 实现验证、bcrypt 与哨兵；用户 DTO 不携带 hash。repo 以数据库约束裁决重复账户并将 unique 失败映射 ErrUserExists；BootstrapAdmin 用数据库事务/锁实现单次初始化；密码更新与全量 Token 吊销同事务。
- [ ] 加双库 `TestConcurrentDuplicateRegistration`（两个请求恰一成功、一个 ErrUserExists）与 `TestConcurrentAdminBootstrap`（仅一个 admin）；先对竞争行为观察 RED，再修实现，不以 mock 代替真实约束。最终相关 service/repo 测试 Expected: PASS。
- [ ] 提交 `feat: add user registration and password services`。

密码长度表驱动测试每个 case 使用启用注册的独立库/service，避免用户名唯一约束干扰。输入使用有效 username/email，密码长度按 bytes 控制，核心断言如下：

```go
cases := []struct {
    length int
    wantErr error
}{
    {11, service.ErrInvalidInput}, {12, nil},
    {72, nil}, {73, service.ErrInvalidInput},
}
// 每个 case：input.Password = strings.Repeat("a", tc.length)
// _, err := svc.Register(t.Context(), input)
// if !errors.Is(err, tc.wantErr) { t.Fatalf("got %v, want %v", err, tc.wantErr) }
```

## Task 5: Token 创建、验证与吊销

**Files:** 创建 internal/service/token.go/token_test.go、internal/repo/token.go/token_test.go；修改 errors.go。

**Interfaces:**

- TokenRepository：`CreateToken(ctx context.Context, token model.Token) (model.Token, error)`、`FindToken(ctx context.Context, id uint64) (model.Token, error)`、`TouchToken(ctx context.Context, id uint64, at time.Time) error`、`ListTokens(ctx context.Context, userID uint64) ([]model.Token, error)`、`RevokeToken(ctx context.Context, userID, tokenID uint64) error`、`RevokeAllTokens(ctx context.Context, userID uint64) error`。
- concrete repo 构造器：`NewTokenRepository(ctx context.Context, db *gorm.DB) (*TokenRepository, error)`。
- `NewTokenService(ctx context.Context, tokens TokenRepository, users UserRepository, now func() time.Time) (*TokenService, error)`；`Issue(ctx context.Context, userID uint64, input TokenInput) (IssuedToken, error)`、`Authenticate(ctx context.Context, raw string) (Identity, error)`、`List(ctx context.Context, userID uint64) ([]TokenView, error)`、`Revoke(ctx context.Context, userID, tokenID uint64) error`、`RevokeAll(ctx context.Context, userID uint64) error`。
- TokenInput：Name/Kind strings，ExpiresAt *time.Time，Abilities []string；TokenView：ID uint64、Name/Kind strings、Abilities []string、LastUsedAt/ExpiresAt *time.Time、CreatedAt time.Time；IssuedToken：Token string、Info TokenView；Identity：User UserView、TokenID uint64、Kind string。TokenKindWeb/TokenKindAPI 的值为 `web`/`api`。

- [ ] 先写 `TestTokenFormatAndStoredHash`：Split 后 40 chars、sha256(secret) 等于 repo 行且无明文；`TestAuthenticateMalformed` 覆盖缺分隔/多分隔/非法 ID/长度错；`TestTokenExpiryAtBoundary` 在 expires_at==now 拒绝；`TestDisabledUser`、`TestRevokedToken`、`TestTokenOwnerIsolation`、`TestListOmitsSecrets`、`TestTouchUpdatesLastUsedAt`。
- [ ] Run: `docker compose -f deploy/compose.dev.yaml run --rm dev go test ./internal/service -v`。Expected: RED。
- [ ] 使用 crypto/rand 生成 20 bytes 后 hex 成 40 chars；数据库生成 id 后才组装明文。secret 哈希只用 SHA-256，与已存值常量时间比较。所有验证都拒绝禁用用户，Touch 出错不得吞掉；到期和 ownership 由 service/repo 一起验证。
- [ ] 加两库实际 Issue→Authenticate→Revoke→Authenticate 测试，最后一步必须 ErrUnauthenticated；固定 UTC 时钟不 sleep。Run service/repo 全套 Expected: PASS。
- [ ] 提交 `feat: add compatible hashed bearer tokens`。同步本计划采纳的 Token 细节到 lsky-api-compat 的项目补充说明，保留上游格式契约。

`TestRevokedToken` 用实际 repo 与一个 enabled 用户，先 Issue 得到 issued；使用同一用户 ID 与 `issued.Info.ID` 调 Revoke，然后验证：

```go
_, err := svc.Authenticate(t.Context(), issued.Token)
if !errors.Is(err, service.ErrUnauthenticated) {
    t.Fatalf("revoked token authenticated: %v", err)
}
```

## Task 6: 原生 HTTP API 与权限边界

**Files:** 创建 internal/http/router.go/response.go/middleware.go 与 native/auth.go/token.go、各测试与 testdata；创建 docs/openapi.yaml。

**Interfaces:** 消费 UserService/TokenService 的业务方法，通过 HTTP 层本地窄接口调用；`NewRouter(ctx context.Context, deps Dependencies) (http.Handler, error)`，Dependencies 字段为 Users（含 Register/VerifyCredentials/ChangePassword 的窄接口）、Tokens（含 Issue/Authenticate/List/Revoke 的窄接口）、Logger *slog.Logger、Server config.Server、Now func() time.Time、Health func(context.Context) error。接口方法类型与 Tasks 4/5 完全相同。Health 在 CLI 注入 sqlDB.PingContext，不让 HTTP 持有具体 repo。native handler 固定 Gin 签名，从 request.Context() 调 service。

- [ ] 写 HTTP 行为测试：`TestLoginMeLogout` 的成功/撤销；`TestRegisterCannotEscalateRole`（拒绝未知 role/group_id 字段）；`TestCrossUserTokenRevoke`（404 或本计划统一 403，选择 403）；`TestTokenListNoSecret`；`TestNativeEnvelopeAndErrorCodes`；`TestPasswordChangeInvalidatesCurrentToken`；`TestRateLimitRejectsFourthAttempt`；`TestForwardedHeaderCannotBypassLimit`。响应按下面表逐字段断言。
- [ ] Run: `docker compose -f deploy/compose.dev.yaml run --rm dev go test ./internal/http/... -v`。Expected: RED。
- [ ] 实现 JSON 参数验证、统一错误映射、Bearer 鉴权与受限 IP 计数。限流器采用标准库 mutex+有限窗口缓存，过期清理，不加入 x/time。Error logger 使用安全错误分类而非任意 err.Error()，尤其不记录驱动 DSN。原生失败 data=null，列表成功无项 data=[]，与蓝空的空对象约定分开。
- [ ] 将精确 HTTP DTO、响应、错误码与安全 scheme 写入 OpenAPI 3.0.3；secret 仅出现在登录/创建 Token 的响应 schema。健康检查 `GET /healthz` 返回 native 外壳与 data.status=ok，检查 DB 可用；未知 API 路由返回 JSON 404，本阶段不做 SPA fallback。
- [ ] Run HTTP 全套 Expected: PASS；以 SQLite/PG 真 repo 再验证完整用户+Token HTTP 流程。提交 `feat: expose native authentication and token APIs`。

`TestRegisterCannotEscalateRole` 在真实 Router 上发送含 `"role":"admin"` 的注册 JSON；所有合法身份字段齐全。对 httptest.ResponseRecorder 的断言：

```go
if response.Code != http.StatusBadRequest {
    t.Fatalf("admin input status=%d, want 400", response.Code)
}
var body struct { Code int `json:"code"` }
if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil { t.Fatal(err) }
if body.Code != 10001 { t.Fatalf("code=%d, want 10001", body.Code) }
```

| 路由 | 输入 | 成功 data 与 HTTP | 主要失败 |
| --- | --- | --- | --- |
| POST /api/auth/register | username,email,password | UserView；201 | 参数 400/10001，关闭 403/30001，重复 409/30002 |
| POST /api/auth/login | email,password | `{token,user,expires_at}`；200 | 凭证错 401/20002，第四次 429/30003 |
| GET /api/auth/me | Bearer | UserView；200 | 缺失/失效 401/20001 |
| POST /api/auth/logout | Bearer | null；200 | 缺失/失效 401/20001 |
| PATCH /api/auth/password | current_password,new_password | null；200 | 当前密码错 401/20002；参数 400/10001 |
| GET /api/tokens | Bearer | TokenView[]；200 | 401/20001 |
| POST /api/tokens | name,expires_at 可空 | `{token,info}`；201，kind 强制 api、abilities 固定 `["*"]` | 参数 400/10001 |
| DELETE /api/tokens/{id} | Bearer | null；200 | 他人 Token 403/20003，非本人且不存在统一 403，避免泄露他人记录 |

UserView 的 HTTP 字段全部 snake_case，时间 UTC RFC3339；expires_at 可 null。HTTP status 与业务 code 分开定义；成功 code=0、message=ok；没有 `/api/v1` 占位响应。

## Task 7: 启动、迁移与管理员 CLI

**Files:** 创建 internal/cli/serve.go/migrate.go/admin.go 与测试；修改 root.go、main.go、README.md、deploy/config.example.yaml。

**Interfaces:** 消费 Tasks 2–6 的入口与 service；`cli.Execute` 签名保持 Task 1 不变。组合根只在 CLI 持有具体 repo；`serve` 调 config.Load→repo.Open→migrate.Check→构造 service→NewRouter→http.Server；`migrate` 显式 Up。

- [ ] 写 `TestServeRefusesPendingMigrations`（临时未迁移 DB）、`TestMigrateCLIIsIdempotent`、`TestInitAdminCLIReadsPasswordFromStdin`、`TestResetPasswordRevokesTokens`、`TestShutdownClosesResources`（取消 context 后请求停止，DB 关闭）。stdin 注入用 Cobra 的 SetIn，给 Execute 增加包内 executeWithInput 辅助，public 签名不变。
- [ ] Run: `docker compose -f deploy/compose.dev.yaml run --rm dev go test ./internal/cli -v`。Expected: RED，对应子命令或生命周期行为缺失。
- [ ] 实现 serve/migrate/init-admin/reset-password；密码不通过 flag、默认值、环境示例、日志返回。admin 初始化不修改已有账户角色；reset 调 UserService；stdout 只写不含秘密的操作结果。main 用 signal.NotifyContext 支持 SIGINT/SIGTERM。
- [ ] 用 http.Server 的 ReadHeaderTimeout=5 s、ShutdownTimeout=10 s（配置默认补齐）实现平滑关闭。无读取完整请求正文的通用日志。跑上述测试与实际容器取消/重启流程 Expected: PASS。
- [ ] README 记录初始化→登录→Token→吊销的本地操作；没有尚未实现的 import-lsky 命令。提交 `feat: wire server lifecycle and admin commands`。

## Task 8: 全量验收与开工记录

**Files:** 创建 docs/development.md；补齐 Makefile、compose.dev.yaml、测试与 .golangci.yml；更新 README.md、spec.md 的已采纳补充设计和适用项目 Skill，记录精确锁文件。

**Interfaces:** 所有已有 task 接口；不新增业务抽象。提供 `make test`、`make test-integration`、`make lint`、`make build`，对应下面相同命令。

本任务的 Go 与 lint 命令在开发 Linux 容器内执行，等价于 `docker compose -f deploy/compose.dev.yaml run --rm dev <command>`；lint 工具版本须单独验证。此前各 Task 的测试先失败再通过，并把两个结果写进 m1-progress.md；本任务不补写未运行的历史结果。

- [ ] 执行 `go mod tidy`、`go mod verify` 与 `go test -race ./...`。Expected: exit 0、全部已实现包绿色；单测与双库集成都要实际执行，不将 skip 当成功。
- [ ] 启动隔离 PostgreSQL 18.6 测试 service，DSN 通过环境注入。`go test -race ./internal/migrate ./internal/repo ./internal/http/... ./internal/cli -count=1 -v`。Expected: 两个 driver 的命名 subtests 均 PASS；无数据库锁错误、race 或泄密输出。测试库必须临时独立 schema/database，不操作生产库。
- [ ] Run: `golangci-lint run ./...`（实测 version 必须 2.14.0）；`go build -trimpath -o /tmp/imgnest ./cmd/imgnest`。Expected: lint 无错误、build exit 0。启用 spec 指定 govet/errcheck/staticcheck/revive/gosec，配置用 v2 格式。
- [ ] 真实 smoke：分别 SQLite 与 PG 执行 migrate→init-admin→serve→login→me→create api token→list→revoke→旧 api token 请求 401；容器重启后数据保留。测试脚本捕获 Token 但不打印，日志负向断言密码/Token/DSN 值均不出现。
- [ ] Run: `docker compose -f deploy/compose.dev.yaml down`。Expected: 仅本项目开发/测试容器与网络停止并移除；保留数据卷，停止服务不冒充删除测试或用户数据。
- [ ] 对照计划书进行完整审查：此阶段只验收 M1，不声称 WebP、图床前端或蓝空迁移完成。按选择的工作流进行独立 code review，必要修复先 RED→GREEN；记录实际命令、结果及未验证项。提交 `docs: document verified M1 development workflow`，生成可审阅的分支变更后交付。

## 阶段完成条件

- SQLite 和 PostgreSQL 均可从空库迁移、重复执行、启动服务、创建管理员、登录、创建/撤销 Token、修改/重置密码与重启保留数据。
- 用户状态、跨用户权限、唯一性与过期边界由真实 repo 测试证明；service 不依赖具体持久化实现。
- 环境、锁文件、OpenAPI、README、默认配置一致；没有 AutoMigrate 或明文凭证入库/日志。
- M1 运行命令与结果已记录，race/lint/build 实际通过；审查结论已处理。
- M2 前置待定项继续留在开工审查，不能靠占位实现提前宣称解决。

## 执行交接

建议由当前会话逐任务实现并在末尾进行独立审查；M1 的接口相互依赖较多，保持一个实施上下文更省重复阅读。也可由用户明确选择逐任务子代理实施/审查。

先审阅本计划及补充设计，再开始实施。当前未建立 Git 仓库、未安装产品依赖、未写业务代码，所有测试输出仍是预期结果。
