# Random Image Link Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 相册所有者可为一个相册生成匿名链接 `/random/<uid>/<token>`，每次访问 307 跳转到相册内随机一张图。

**Architecture:** 新表 `random_links` 加 `users.public_id`；`RandomLinkService` 每次请求查库解析链接，候选图走 `RandomPool` 接口（本轮只有内存实现，TTL 60 秒 + 主动失效）；公开路由和四个管理路由挂在 native handler 上，公开路由用独立限流实例。

**Tech Stack:** Go（gin、gorm、PostgreSQL + SQLite）、Vue 3 + TypeScript（web-vben）、vitest。不新增任何依赖。

**Spec:** `docs/superpowers/specs/2026-10-07-random-image-link-design.md`

## Global Constraints

- 不新增 Go 或 npm 依赖（`docs/versions.md`）；Redis 不进 `go.mod`。
- 分层单向 `http → service → repo`；service 只依赖接口。
- 迁移只新增 `0007_random_links.sql`（postgres、sqlite 各一份），不改 `0001`–`0006`，不用 `AutoMigrate`。
- 每个对外函数第一个参数是 `context.Context`；错误用 `%w` 包装；业务错误用现有哨兵值，由 `native.fail` 统一映射。
- `uid`、`token`、GPS、密钥不进日志；公开接口不返回 EXIF 和图片元数据。
- URL 不入库，实时拼接：直链用 `service.objectURL(backend.BaseURL, key)`，对象 Key 为 `{path}.{ext}` / `{path}.webp`。
- 只有 `state = active` 的图参与随机。
- 常量（全计划通用）：`uid` 10 位 base62；`token` 24 位 base62；base62 字母表 `0-9A-Za-z`；池 TTL `60 * time.Second`；池上限 `5000`；随机路由限流 `600` 次/分钟/IP，记录上限 `16384`；生成冲突重试 `5` 次。
- 提交信息用 Conventional Commits，结尾带 `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`。
- 测试命令（都在 dev 容器里跑）：
  - 单包：`docker compose -f deploy/compose.dev.yaml run --rm dev go test -race -count=1 ./internal/<pkg>/ -run <Name>`
  - 全量：`make test`；lint：`make lint`；前端：`make fe-test`、`make fe-lint`
- 写 Go 代码前按 `AGENTS.md` 加载 `test-driven-development`、`golang-code-style`、`golang-error-handling`、`golang-database`、`golang-concurrency`、`golang-testing`；写前端前加载 `vue-best-practices`、`vue-testing-best-practices`。

## Review Focus

1. **SQLite 复用相册 ID**：删掉相册 A（id=7）后新建的相册 B 可能拿到同一个 id；B 的新链接不能在 60 秒内抽到 A 的旧池。→ Task 4 `TestPutNewLinkInvalidatesPool`。
2. **图片缺某个版本**：`has_webp=false` 时默认请求要回退原图；`has_original=false` 时 `format=original` 要回退 WebP；两者都没有的图不进候选。→ Task 2 `TestCandidatesSkipUnusable`、Task 4 `TestPickVersionFallback`。
3. **路径含空格、中文、`#`、`?`**：`Location` 必须逐段转义，不能截断或变成查询串。→ Task 4 `TestPickEscapesPath`。
4. **畸形 `uid`/`token`**（超长、含 `/`、`%00`、非 base62）：直接 404，不查库，不 500。→ Task 6 `TestRandomRejectsMalformedWithoutLookup`。
5. **同一相册并发首次创建**（双击启用）：第二个请求撞 `album_id` 唯一索引，应返回已存在的链接，不能 500。→ Task 4 `TestPutConcurrentCreateReturnsExisting`。

---

### Task 1: 迁移与模型

**Files:**
- Create: `internal/migrate/postgres/0007_random_links.sql`、`internal/migrate/sqlite/0007_random_links.sql`
- Create: `internal/model/random_link.go`
- Modify: `internal/model/user.go`（`User` 加字段）
- Test: `internal/migrate/migrate_test.go`

**Interfaces:**
- Produces:

```go
// internal/model/random_link.go
type RandomLink struct {
	ID        uint64 `gorm:"primaryKey"`
	UserID    uint64
	AlbumID   uint64
	Token     string
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// RandomCandidate carries only what a redirect URL needs.
type RandomCandidate struct {
	StorageID   uint64
	Path        string
	Ext         string
	HasWebP     bool `gorm:"column:has_webp"`
	HasOriginal bool
}

// internal/model/user.go — added to User
PublicID *string
```

- [ ] **Step 1: 写失败测试** — 在 `migrate_test.go` 按文件里现有的按驱动循环的写法加 `TestRandomLinksMigration`：`Up` 之后断言
  - `random_links` 表存在；
  - 向 `users` 插入两行 `public_id` 为 NULL 成功，再插入两行相同非空 `public_id` 第二行失败；
  - `random_links` 中重复 `album_id` 失败、重复 `token` 失败；
  - 迁移前已存在的 `users` 行保留且 `public_id IS NULL`。

- [ ] **Step 2: 运行确认失败** — `... go test -race -count=1 ./internal/migrate/ -run TestRandomLinksMigration`，预期 FAIL（表不存在）。

- [ ] **Step 3: 写两份迁移 SQL**
  - `ALTER TABLE users ADD COLUMN public_id TEXT;`
  - `CREATE UNIQUE INDEX users_public_id_key ON users (public_id);`（两种库都允许多个 NULL）
  - `CREATE TABLE random_links`：`id` 自增主键（写法照抄各自驱动 `0002_image_core.sql` 里 `albums` 表的主键与时间列类型）、`user_id BIGINT NOT NULL REFERENCES users(id)`、`album_id BIGINT NOT NULL REFERENCES albums(id)`、`token TEXT NOT NULL`、`enabled BOOLEAN NOT NULL DEFAULT TRUE`、`created_at`、`updated_at`。
  - 唯一索引 `random_links_album_id_key (album_id)`、`random_links_token_key (token)`；普通索引 `random_links_user_id_idx (user_id)`。

- [ ] **Step 4: 加模型**（上面 Interfaces 的三处）。

- [ ] **Step 5: 运行** — `./internal/migrate/ ./internal/model/ ./internal/repo/`，预期全部 PASS（确认 `User` 加字段没破坏现有 repo 测试）。

- [ ] **Step 6: 提交** — `feat: add random link schema and models`

---

### Task 2: 持久层

**Files:**
- Create: `internal/repo/random_link.go`、`internal/repo/random_link_test.go`
- Modify: `internal/model/errors.go`（加 `ErrRandomLinkExists`）
- Modify: `internal/repo/user.go`（加 `EnsurePublicID`）、`internal/repo/user_test.go`
- Modify: `internal/repo/album.go:166`（`DeleteOwned` 事务内删链接）、`internal/repo/album_test.go`

**Interfaces:**
- Consumes: Task 1 的 `model.RandomLink`、`model.RandomCandidate`、`User.PublicID`。
- Produces:

```go
type RandomLinkRepository struct{ db *gorm.DB }
func NewRandomLinkRepository(ctx context.Context, db *gorm.DB) (*RandomLinkRepository, error)

// FindByAlbum reports model.ErrNotFound when the album has no link.
func (r *RandomLinkRepository) FindByAlbum(ctx context.Context, albumID uint64) (model.RandomLink, error)
// Create reports album_id or token unique violations as model.ErrRandomLinkExists.
func (r *RandomLinkRepository) Create(ctx context.Context, link model.RandomLink) (model.RandomLink, error)
// Update changes only "enabled" and/or "token"; other keys are ErrInvalidInput.
func (r *RandomLinkRepository) Update(ctx context.Context, albumID uint64, values map[string]any) (model.RandomLink, error)
// DeleteByAlbum is idempotent.
func (r *RandomLinkRepository) DeleteByAlbum(ctx context.Context, albumID uint64) error
// FindByToken returns the link with its owner row.
func (r *RandomLinkRepository) FindByToken(ctx context.Context, token string) (model.RandomLink, model.User, error)
// Candidates returns up to limit usable active images in random order.
func (r *RandomLinkRepository) Candidates(ctx context.Context, albumID uint64, limit int) ([]model.RandomCandidate, error)

// internal/model/errors.go
var ErrRandomLinkExists = errors.New("random link exists")

// internal/repo/user.go
// EnsurePublicID returns the stored public ID, generating one on first use.
func (r *UserRepository) EnsurePublicID(ctx context.Context, userID uint64, generate func() (string, error)) (string, error)
```

- [ ] **Step 1: 写失败测试**（用 `image_test.go` 的 `newImageFixture` 建用户、存储、图片；按文件里现有方式对 sqlite 和 postgres 各跑一遍）
  - `TestRandomLinkCRUD`：`Create` 后 `FindByAlbum` 取回同一 token；`Update` 改 `enabled=false` 和新 `token` 生效；`DeleteByAlbum` 两次都返回 nil；之后 `FindByAlbum` 是 `model.ErrNotFound`。
  - `TestRandomLinkCreateDuplicate`：同一 `album_id` 第二次 `Create` 返回 `errors.Is(err, model.ErrRandomLinkExists)`；不同相册相同 `token` 也是。
  - `TestRandomLinkFindByToken`：返回的 `User.ID`、`User.Status`、`User.PublicID` 正确；未知 token 是 `model.ErrNotFound`。
  - `TestCandidatesOnlyActive`：相册内 3 张 active、1 张 trash、1 张 pending，另一个相册 1 张 active → 返回 3 条，字段与图片行一致。
  - `TestCandidatesSkipUnusable`：`has_original=false AND has_webp=false` 的 active 图不返回。
  - `TestCandidatesLimit`：7 张图 `limit=5` 返回 5 条且互不相同；`limit<1` 返回 `model.ErrInvalidInput`。
  - `TestEnsurePublicID`（`user_test.go`）：首次调用存入 `generate` 的值；第二次调用返回同值且不调用 `generate`；`generate` 前两次返回另一用户已占用的值、第三次返回新值 → 成功；连续 5 次都冲突 → 返回错误且该用户 `public_id` 仍为 NULL。
  - `TestAlbumDeleteRemovesRandomLink`（`album_test.go`）：有链接的相册 `DeleteOwned` 后 `random_links` 无该行。

- [ ] **Step 2: 运行确认失败** — `./internal/repo/ -run 'RandomLink|Candidates|EnsurePublicID|AlbumDeleteRemoves'`，预期编译失败。

- [ ] **Step 3: 实现 `RandomLinkRepository`**
  - 错误包装沿用本包的 `repositoryError` / `checkRecordID` / `checkDatabase`。
  - 唯一冲突的识别方式照 `repo/user.go` 的 `userWriteError`，映射为 `model.ErrRandomLinkExists`。
  - `Candidates`：`SELECT storage_id, path, ext, has_webp, has_original FROM images WHERE album_id = ? AND state = 'active' AND (has_original OR has_webp) ORDER BY RANDOM() LIMIT ?`（`RANDOM()` 在两种库里都可用）。
  - `FindByToken`：先按 `token` 取链接，再按 `user_id` 取用户；两步任一未找到都返回 `model.ErrNotFound`。

- [ ] **Step 4: 实现 `EnsurePublicID`** — 事务内 `lockUser`；已有值直接返回；否则最多 5 次调用 `generate` 并 `UPDATE`，唯一冲突则重试。postgres 下事务内语句失败会使事务中止，所以每次尝试用 `tx.SavePoint` / `RollbackTo` 包住。

- [ ] **Step 5: 改 `DeleteOwned`** — 在 `tx.Delete(&model.Album{}...)` 之前加 `tx.Delete(&model.RandomLink{}, "album_id = ?", albumID)`。

- [ ] **Step 6: 运行** — `./internal/repo/`，预期全部 PASS。

- [ ] **Step 7: 提交** — `feat: persist random links and user public ids`

---

### Task 3: 内存候选池

**Files:**
- Create: `internal/randompool/memory.go`、`internal/randompool/memory_test.go`

**Interfaces:**
- Consumes: `model.RandomCandidate`。
- Produces:

```go
// Package randompool caches per-album redirect candidates.
type Memory struct{ /* sync.RWMutex, map[uint64]entry, now */ }
func NewMemory(now func() time.Time) *Memory
// Get returns a shared slice the caller must not modify.
func (m *Memory) Get(ctx context.Context, albumID uint64) ([]model.RandomCandidate, bool, error)
func (m *Memory) Set(ctx context.Context, albumID uint64, items []model.RandomCandidate, ttl time.Duration) error
func (m *Memory) Invalidate(ctx context.Context, albumID uint64) error
```

- [ ] **Step 1: 写失败测试**
  - `TestMemoryHitAndExpiry`：注入时钟；`Set(ttl=60s)` 后 `Get` 命中；时钟前进 59s 仍命中；前进到 60s 整返回 `ok=false`。
  - `TestMemoryInvalidate`：`Invalidate` 后 `ok=false`；对不存在的相册 `Invalidate` 返回 nil。
  - `TestMemoryStoresEmpty`：`Set` 空切片后 `Get` 返回 `ok=true`、长度 0（空相册也要缓存，避免每次请求打库）。
  - `TestMemorySetCopies`：`Set` 之后修改传入的切片，`Get` 结果不变。
  - `TestMemorySweepsExpired`：写入 2000 个已过期相册后再 `Set` 一个，内部 map 长度为 1。
  - `TestMemoryConcurrent`：32 个 goroutine 混合 `Get`/`Set`/`Invalidate`，`-race` 下无报告。
  - `TestMemoryContextCancelled`：已取消的 ctx 下三个方法都返回 `ctx.Err()`。

- [ ] **Step 2: 运行确认失败** — `./internal/randompool/`，预期编译失败。

- [ ] **Step 3: 实现** — 过期判定为 `!now.Before(entry.until)`；`Set` 时若 map 长度超过 1024 则顺手清掉所有已过期项。

- [ ] **Step 4: 运行** — `./internal/randompool/`，预期 PASS。

- [ ] **Step 5: 提交** — `feat: add in-memory random candidate pool`

---

### Task 4: RandomLinkService

**Files:**
- Create: `internal/service/random_link.go`、`internal/service/random_link_test.go`

**Interfaces:**
- Consumes: Task 2 的 repo 方法、Task 3 的池方法（都经由下面的接口）、现有 `AlbumStore.FindOwned`、`StorageRepository.Find`、`objectURL`。
- Produces:

```go
const (
	RandomPoolTTL   = 60 * time.Second
	RandomPoolLimit = 5000
)

type RandomPoolInvalidator interface {
	Invalidate(ctx context.Context, albumID uint64) error
}
type RandomPool interface {
	RandomPoolInvalidator
	Get(ctx context.Context, albumID uint64) ([]model.RandomCandidate, bool, error)
	Set(ctx context.Context, albumID uint64, items []model.RandomCandidate, ttl time.Duration) error
}
type RandomLinkRepository interface { /* the six RandomLinkRepository methods from Task 2 */ }
type PublicIDStore interface {
	EnsurePublicID(ctx context.Context, userID uint64, generate func() (string, error)) (string, error)
}
type RandomStorages interface {
	Find(ctx context.Context, id uint64) (model.Storage, error)
}
type RandomLinkDependencies struct {
	Links    RandomLinkRepository
	Albums   AlbumStore
	Users    PublicIDStore
	Storages RandomStorages
	Pool     RandomPool
	Logger   *slog.Logger
	// Intn picks an index in [0, n); nil uses math/rand/v2.
	Intn func(n int) int
}
type RandomLinkView struct {
	Enabled   bool      `json:"enabled"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
}
type RandomLinkService struct{ /* deps */ }
func NewRandomLinkService(ctx context.Context, deps RandomLinkDependencies) (*RandomLinkService, error)
// Get returns nil when the album has no link.
func (s *RandomLinkService) Get(ctx context.Context, ownerID, albumID uint64) (*RandomLinkView, error)
func (s *RandomLinkService) Put(ctx context.Context, ownerID, albumID uint64, enabled bool) (RandomLinkView, error)
func (s *RandomLinkService) Reset(ctx context.Context, ownerID, albumID uint64) (RandomLinkView, error)
func (s *RandomLinkService) Delete(ctx context.Context, ownerID, albumID uint64) error
// Pick returns the redirect target; every lookup miss is ErrNotFound.
func (s *RandomLinkService) Pick(ctx context.Context, uid, token string, original bool) (string, error)
// ValidRandomSegment reports whether value is exactly n base62 characters.
func ValidRandomSegment(value string, n int) bool
```

- [ ] **Step 1: 写失败测试**（用手写 fake 实现各接口，风格照 `album_test.go` 的 `newAlbumFixture`）
  - 管理操作：
    - `TestGetWithoutLinkIsNil`；`TestGetForeignAlbum`：`Albums.FindOwned` 返回 `ErrForbidden` 时原样透传，且不触达 `Links`。
    - `TestPutCreates`：返回 `Enabled=true`，`Path` 匹配 `^/random/[0-9A-Za-z]{10}/[0-9A-Za-z]{24}$`，且其中的 uid 等于 `EnsurePublicID` 的返回值。
    - `TestPutUpdatesEnabledKeepsToken`：已有链接 `Put(false)` 后 token 不变。
    - `TestPutNewLinkInvalidatesPool`：新建时 `Pool.Invalidate(albumID)` 被调用一次。
    - `TestPutConcurrentCreateReturnsExisting`：`Links.Create` 返回 `model.ErrRandomLinkExists` 且随后 `FindByAlbum` 能取到 → 返回该已存在链接，无错误。
    - `TestPutTokenCollisionRetries`：`Create` 前两次因 token 冲突失败（`FindByAlbum` 仍是 NotFound）第三次成功 → 成功；连续 5 次失败 → 返回非 nil 错误。
    - `TestResetChangesToken`：新旧 token 不同；无链接时 `errors.Is(err, ErrNotFound)`。
    - `TestDeleteIdempotentAndInvalidates`。
  - `Pick`：
    - `TestPickUniformNotFound`：以下每种都 `errors.Is(err, ErrNotFound)`——未知 token、`uid` 与所有者 `PublicID` 不符、所有者 `PublicID` 为 nil、链接 `Enabled=false`、用户 `Status=disabled`、候选为空。
    - `TestPickUsesPool`：池命中时不调用 `Links.Candidates`；未命中时调用一次 `Candidates(albumID, 5000)` 并 `Set(..., 60*time.Second)`。
    - `TestPickVersionFallback`（表驱动，`BaseURL="https://cdn.example"`，`Path="2026/10/a"`，`Ext="jpg"`）：

      | HasWebP | HasOriginal | original | 期望 |
      | --- | --- | --- | --- |
      | true | true | false | `https://cdn.example/2026/10/a.webp` |
      | true | true | true | `https://cdn.example/2026/10/a.jpg` |
      | false | true | false | `https://cdn.example/2026/10/a.jpg` |
      | true | false | true | `https://cdn.example/2026/10/a.webp` |

    - `TestPickEscapesPath`：`Path="相册 1/a#b?c"`、`BaseURL="https://cdn.example/"` → `https://cdn.example/%E7%9B%B8%E5%86%8C%201/a%23b%3Fc.webp`。
    - `TestPickSelectsByIntn`：3 个候选，`Intn` 固定返回 2 → 取第 3 个。
    - `TestPickPoolErrorsDegrade`：`Pool.Get` 返回错误时按未命中处理并成功返回；`Pool.Set` 返回错误时仍成功返回。
    - `TestPickStorageMissing`：`Storages.Find` 失败 → `errors.Is(err, ErrNotFound)`。
  - `TestValidRandomSegment`：`("abcDEF0123", 10)` 为真；长度不符、含 `-`、`/`、`%`、空串、非 ASCII 为假。

- [ ] **Step 2: 运行确认失败** — `./internal/service/ -run 'RandomLink|TestPut|TestPick|TestGet|TestReset|TestDelete|ValidRandom'`，预期编译失败。

- [ ] **Step 3: 实现**
  - 构造函数：`Links`/`Albums`/`Users`/`Storages`/`Pool`/`Logger` 任一为 nil 返回 `ErrInvalidInput`。
  - 所有管理操作先 `Albums.FindOwned(ctx, ownerID, albumID)`，错误原样包装返回。
  - base62 生成：`crypto/rand` 取字节，丢弃 `>= 248` 的字节后对 62 取模（拒绝采样，避免偏差）。先看 `service/token.go` 里现有的随机串生成，能复用就复用。
  - `Pick` 顺序：`ValidRandomSegment(uid,10)` 与 `(token,24)` → `Links.FindByToken` → 用 `subtle.ConstantTimeCompare` 比较 `uid` 与 `*user.PublicID` → 检查 `link.Enabled` 与 `user.Status == model.UserStatusEnabled` → 取池 → 选图 → `Storages.Find` → `objectURL`。
  - 池错误只用 `Logger.WarnContext` 记录 `album_id`，不记 `uid`/`token`。
  - 仓库返回的 `model.ErrNotFound` 按本包现有做法映射到 `ErrNotFound`（看 `service/errors.go` 里两者的关系）。

- [ ] **Step 4: 运行** — `./internal/service/`，预期全部 PASS。

- [ ] **Step 5: 提交** — `feat: add random link service`

---

### Task 5: 图片变更时失效候选池

**Files:**
- Modify: `internal/service/image.go:105`（`ImageDependencies` 加可选字段）
- Modify: `internal/service/upload.go`（`Upload` 成功处）、`internal/service/image_album.go:39`（`SetAlbum`）、`internal/service/trash.go:107,185`（`Trash`、`Restore`）、`internal/service/image_admin.go:75`（`AdminTrash`，若它不经过 `Trash`）
- Test: `internal/service/random_invalidate_test.go`

**Interfaces:**
- Consumes: Task 4 的 `RandomPoolInvalidator`。
- Produces: `ImageDependencies.RandomPool RandomPoolInvalidator`（nil 表示不失效）；私有方法 `func (s *ImageService) invalidateRandom(ctx context.Context, albumIDs ...uint64)`。

- [ ] **Step 1: 写失败测试** — 用记录调用的 fake `RandomPoolInvalidator`，沿用 `image_album_test.go` / `upload_test.go` 现有的 fixture：
  - `TestUploadInvalidatesAlbumPool`：上传到相册 5 → 记录 `[5]`；不带相册上传 → 无调用。
  - `TestSetAlbumInvalidatesBothPools`：从 5 移到 6 → 记录包含 5 和 6；从 5 移出（albumID=0）→ 只有 5。
  - `TestTrashAndRestoreInvalidate`：各记录图片所在相册一次。
  - `TestAdminTrashInvalidates`。
  - `TestInvalidateFailureDoesNotFailOperation`：fake 返回错误时 `Trash` 仍返回 nil。
  - `TestNilRandomPoolIsNoop`：不注入时以上操作照常成功。

- [ ] **Step 2: 运行确认失败** — `./internal/service/ -run 'Invalidat|NilRandomPool'`，预期 FAIL。

- [ ] **Step 3: 实现** — `invalidateRandom` 跳过 0 和 nil 池，忽略返回的错误（60 秒 TTL 兜底）；只在各操作**成功返回之前**调用，失败路径不调用。`SetAlbum` 需要在改动前读到旧 `AlbumID`。

- [ ] **Step 4: 运行** — `./internal/service/`，预期全部 PASS。

- [ ] **Step 5: 提交** — `feat: invalidate random pools on image changes`

---

### Task 6: HTTP 路由、限流与装配

**Files:**
- Create: `internal/http/native/ratelimit.go`、`internal/http/native/ratelimit_test.go`
- Modify: `internal/http/native/auth.go:46-117`（`Handler` 改用新限流器，行为不变）
- Create: `internal/http/native/random.go`、`internal/http/native/random_test.go`
- Modify: `internal/http/router.go`（`Dependencies.RandomLinks` 与注册）
- Modify: `internal/cli/serve.go`、`internal/cli/image_runtime.go`（装配）
- Test: `internal/http/router_random_integration_test.go`

**Interfaces:**
- Consumes: Task 3 `randompool.NewMemory`、Task 4 `RandomLinkService`、Task 5 `ImageDependencies.RandomPool`。
- Produces:

```go
// internal/http/native/ratelimit.go
type fixedWindowLimiter struct{ /* mu, windows map[string]window, max, capacity int, now */ }
func newFixedWindowLimiter(max, capacity int, now func() time.Time) *fixedWindowLimiter
// allow reports whether key may proceed in the current one-minute window.
func (l *fixedWindowLimiter) allow(key string) bool

// internal/http/native/random.go
type RandomLinks interface {
	Get(ctx context.Context, ownerID, albumID uint64) (*service.RandomLinkView, error)
	Put(ctx context.Context, ownerID, albumID uint64, enabled bool) (service.RandomLinkView, error)
	Reset(ctx context.Context, ownerID, albumID uint64) (service.RandomLinkView, error)
	Delete(ctx context.Context, ownerID, albumID uint64) error
	Pick(ctx context.Context, uid, token string, original bool) (string, error)
}
func (h *Handler) RegisterRandomLinkRoutes(ctx context.Context, router gin.IRouter, links RandomLinks) error

// internal/http/router.go — added to Dependencies
RandomLinks native.RandomLinks
```

路由：

| 方法 | 路径 | 鉴权 |
| --- | --- | --- |
| GET, HEAD | `/random/:uid/:token` | 无，独立限流（600/分钟/IP，上限 16384） |
| GET | `/api/albums/:id/random-link` | `h.authenticate` |
| PUT | `/api/albums/:id/random-link` | `h.authenticate` |
| POST | `/api/albums/:id/random-link/reset` | `h.authenticate` |
| DELETE | `/api/albums/:id/random-link` | `h.authenticate` |

- [ ] **Step 1: 限流器的失败测试**（`ratelimit_test.go`）
  - `TestLimiterWindow`：`max=3` 时前 3 次 `allow` 为真、第 4 次为假；时钟前进 1 分钟后恢复。
  - `TestLimiterCapacity`：`capacity=2` 时第 3 个不同 key 被拒绝；已有 key 过期后新 key 可进入。
  - `TestLimitersAreIndependent`：把一个实例打满不影响另一个实例。

- [ ] **Step 2: 实现限流器并替换 `Handler` 里的 `mu`/`limits`** — `NewHandler` 里创建 `newFixedWindowLimiter(3, 4096, now)`；`h.rateLimit` 的 key（`c.FullPath()+":"+c.ClientIP()`）与 429 响应体不变。运行 `./internal/http/...`，预期现有测试全部 PASS。

- [ ] **Step 3: handler 的失败测试**（`random_test.go`，用 fake `RandomLinks` 加 `httptest`）
  - `TestRandomRedirects`：`Pick` 返回 `https://cdn.example/a.webp` → 状态 `307`，`Location` 等于该值，`Cache-Control: no-store`，响应体不含 JSON 元数据；`HEAD` 同样 `307`。（`no-store` 由 `router.go` 的全局中间件设置，此断言放在 Step 6 的集成测试里；这里断言 handler 没有把它改掉。）
  - `TestRandomFormat`：无参数 → `Pick(..., false)`；`?format=original` → `true`；`?format=png` → `400` 且 `Pick` 未被调用。
  - `TestRandomNotFound`：`Pick` 返回 `service.ErrNotFound` → `404`，响应体与一个未知路径的 404 字段相同（`code`、`message`）。
  - `TestRandomRejectsMalformedWithoutLookup`：`uid` 为 9 位、11 位、含 `-`；`token` 为 23 位、含 `%00`、长度 5000 → 全部 `404` 且 `Pick` 未被调用。
  - `TestRandomRateLimited`：同一 IP 第 601 次请求返回 `429`、`code` 30003；此时对 `/api/auth/login` 的限流计数不受影响。
  - `TestRandomLinkManagement`：未带 Bearer → `401`；`GET` 无链接 → `200` 且 `data` 为 `null`；`PUT {"enabled":true}` → `200` 且 `data.path` 存在；`PUT` 缺 `enabled` 字段或 body 非 JSON → `400`；`reset` → `200`；`DELETE` → `200` 且 `data` 为 `null`；`:id` 为 `0` 或非数字 → `400`；service 返回 `ErrForbidden` 时状态码与 `PATCH /api/albums/:id` 对他人相册的一致。

- [ ] **Step 4: 运行确认失败** — `./internal/http/native/ -run 'TestRandom'`，预期编译失败。

- [ ] **Step 5: 实现 `random.go`** — 管理路由复用 `albumID(c)`、`decode`、`respond`、`fail`、`identity(c).User.ID`；`PUT` 的 body 用 `Enabled *bool` 以区分缺失。公开路由用 `service.ValidRandomSegment` 先校验，再 `c.Redirect(http.StatusTemporaryRedirect, url)`。`router.go` 中 `deps.RandomLinks != nil` 时注册。

- [ ] **Step 6: 集成测试**（`router_random_integration_test.go`，照 `router_album_integration_test.go` 的 `newAlbumHTTPFixture` 搭真实 repo + service + 内存池）
  - `TestRandomLinkEndToEnd`：注册用户 → 建相册 → 上传 2 张图进相册 → `PUT random-link` → 用返回的 `path` 请求 20 次：全部 `307`、`Cache-Control: no-store`，`Location` 都属于这 2 张图的 WebP 直链且两张都出现过。
  - `TestRandomLinkStopsAfterTrash`：把其中一张移入回收站后再请求 20 次，`Location` 只会是另一张；两张都进回收站后是 `404`。
  - `TestRandomLinkResetAndDisable`：`reset` 后旧 path `404`、新 path `307`；`PUT {"enabled":false}` 后 `404`；管理员禁用该用户后 `404`。
  - `TestRandomLinkAlbumDeleted`：删除相册后 `404`。
  - `TestRandomLinkNotLogged`：捕获 logger 输出，不含该 `token` 与 `uid`。

- [ ] **Step 7: 装配**
  - `serve.go`：`pool := randompool.NewMemory(time.Now)`，传给 `newImageServices`（新增一个 `service.RandomPoolInvalidator` 参数，赋给 `ImageDependencies.RandomPool`；CLI 里其他调用点传 nil）。
  - 新增 `newRandomLinkService(ctx, db, pool, logger)`，用 `repo.NewRandomLinkRepository`、`repo.NewAlbumRepository`、`repo.NewUserRepository`、`repo.NewStorageRepository` 组装，结果放进 `httpapi.Dependencies.RandomLinks`。

- [ ] **Step 8: 运行** — `./internal/http/... ./internal/cli/...`，预期全部 PASS。

- [ ] **Step 9: 提交** — `feat: serve random image links`

---

### Task 7: 契约与规范文档

**Files:**
- Modify: `docs/openapi.yaml`（在 `/api/albums/{id}` 之后加两个 path 项，`components.schemas` 加 `RandomLinkView`；另加公开的 `/random/{uid}/{token}`）
- Modify: `docs/spec.md`（新增“随机图片链接”小节）
- Modify: `web-vben/src/api/schema.d.ts`（生成产物）

**Interfaces:**
- Produces: `components["schemas"]["RandomLinkView"]`：`{ enabled: boolean; path: string; created_at: string }`，三个字段都 required。

- [ ] **Step 1: 写 OpenAPI** — `GET`/`PUT`/`DELETE /api/albums/{id}/random-link`、`POST /api/albums/{id}/random-link/reset`、`GET /random/{uid}/{token}`（`format` 查询参数 enum `[original]`，响应 `307` 带 `Location` 头、`400`、`404`、`429`）。写法照同文件里 `/api/albums/{id}` 与 `/i/{storageID}/{key}`。`GET` 的 `data` 用 `nullable`/`oneOf null`，与文件里其他可空 `data` 的写法一致。

- [ ] **Step 2: 生成类型** — `make fe-gen-api`，预期 `schema.d.ts` 出现 `RandomLinkView`，`git diff --stat` 只涉及该文件与 `openapi.yaml`。

- [ ] **Step 3: 更新 `docs/spec.md`** — 新小节写入：路径形状、`uid`/`token` 含义与长度、307 与 `no-store`、默认 WebP 与 `?format=original`、参与范围（不看 `is_public`）、统一 404、限流 600/分钟/IP、池 TTL 60 秒与上限 5000、Redis 仅预留接口。数据表清单与接口清单各加一行。

- [ ] **Step 4: 提交** — `docs: document random image links`

---

### Task 8: 前端

**Files:**
- Modify: `web-vben/src/api/albums.ts`、`web-vben/src/api/albums.test.ts`
- Create: `web-vben/src/components/albums/AlbumRandomLink.vue`、`AlbumRandomLink.test.ts`
- Modify: `web-vben/src/views/AlbumDetailView.vue`
- Modify: `web-vben/src/locales/messages/zh-CN/albums.json`、`en-US/albums.json`

**Interfaces:**
- Consumes: Task 7 的 `components["schemas"]["RandomLinkView"]`。
- Produces:

```ts
// src/api/albums.ts
export type RandomLinkView = components["schemas"]["RandomLinkView"];
export function getRandomLink(albumId: number): Promise<RandomLinkView | null>;
export function putRandomLink(albumId: number, enabled: boolean): Promise<RandomLinkView>;
export function resetRandomLink(albumId: number): Promise<RandomLinkView>;
export function deleteRandomLink(albumId: number): Promise<null>;
```

`AlbumRandomLink.vue`：props `{ albumId: number }`，无 emits，自己加载和维护链接状态。

- [ ] **Step 1: API 的失败测试**（照 `albums.test.ts` 现有 mock `request` 的写法）— 四个函数各断言方法与路径：`GET /api/albums/7/random-link`、`PUT`（body `{"enabled":true}`）、`POST .../reset`、`DELETE`。

- [ ] **Step 2: 实现四个函数**，运行 `make fe-test`，预期新用例 PASS。

- [ ] **Step 3: 组件的失败测试**（mock `@/api/albums`）
  - 无链接时：显示启用开关为关，不显示链接文本。
  - 打开开关：先出现确认提示，文案含 i18n key `albums.randomLink.privacyWarning`；确认后调用 `putRandomLink(7, true)`，随后显示 `window.location.origin + path`。取消则不调用。
  - 已有链接再次开关：不再弹隐私提示，直接调用 `putRandomLink(7, false)`。
  - 复制按钮：调用 `navigator.clipboard.writeText`，参数为完整 URL。
  - 重置：确认后调用 `resetRandomLink(7)`，显示的 URL 变为新值；取消则不调用。
  - 删除：确认后调用 `deleteRandomLink(7)`，回到无链接状态。
  - 接口抛错：显示错误提示，开关回到操作前的状态。
  - `albumId` prop 变化：重新调用 `getRandomLink`。

- [ ] **Step 4: 实现组件** — UI 组件库、确认弹窗、消息提示、`useI18n` 的用法照 `AlbumFormModal.vue`；`<script setup lang="ts">`；请求只经 `src/api/albums.ts`。界面含：启用开关、只读链接框、复制按钮、一行说明“加 `?format=original` 获取原图”、重置按钮、删除按钮。

- [ ] **Step 5: i18n** — 两个语言文件加 `randomLink` 分组：`title`、`enable`、`copy`、`copied`、`formatHint`、`reset`、`resetConfirm`（提示旧链接失效）、`delete`、`deleteConfirm`、`privacyWarning`（相册内所有图片含私有图片都可能被匿名访问）、`loadFailed`、`saveFailed`。

- [ ] **Step 6: 接入 `AlbumDetailView.vue`** — 在相册信息区下方渲染 `<AlbumRandomLink :album-id="albumId" />`。若 `vite.config` 的 dev proxy 列了 `/i` 或 `/t`，同样加上 `/random`。

- [ ] **Step 7: 运行** — `make fe-test && make fe-lint`，预期全部 PASS、无类型或 lint 错误。

- [ ] **Step 8: 提交** — `feat: manage random image links in album view`

---

### Task 9: 整体验证

- [ ] **Step 1:** `make test` — 预期全部 PASS，无 race 报告。
- [ ] **Step 2:** `make lint` — 预期 0 issues。
- [ ] **Step 3:** 覆盖率 — `... go test -race -cover ./internal/randompool/ ./internal/service/`；`randompool` ≥ 80%，`random_link.go` 的函数覆盖用 `go tool cover -func` 确认 ≥ 80%。
- [ ] **Step 4:** `make fe-test && make fe-lint && make release` — 预期构建成功。
- [ ] **Step 5:** 真实二进制冒烟 — `make migrate && make serve`，在浏览器里登录、建相册、上传 2 张图、启用随机链接，用 `curl -sI` 请求链接 5 次确认 `307` 与不同的 `Location`；把一张图移入回收站后确认不再出现；重置后旧链接 `404`。
- [ ] **Step 6:** 按 `AGENTS.md` 加载 `verification-before-completion` 与 `requesting-code-review`，通过后再开 PR。
