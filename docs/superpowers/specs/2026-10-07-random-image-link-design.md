# 相册随机图片链接设计（2026-10-07）

本文记录随机图片功能的设计决定；与 `docs/spec.md` 冲突时以 spec 为准。实现完成后需把本功能同步写入 `docs/spec.md` 与 `docs/openapi.yaml`。

## 目标

相册所有者为一个相册生成一条匿名外链 `/random/{uid}/{token}`，每次访问随机跳转到该相册中的一张图片。用途：博客背景、README 配图、随机壁纸 API。

不在本轮：一条链接对应多个相册、Redis 缓存实现、按尺寸/方向筛选、访问统计。

## 已确认的决定

| 项 | 决定 |
| --- | --- |
| 路径 | `GET\|HEAD /random/:uid/:token` |
| `uid` | 用户的随机公开 ID（`users.public_id`，10 位 base62），不是数字 ID 的编码，不依赖任何密钥 |
| `token` | 标识一条随机链接，24 位 base62，明文存库，可随时查看、可重置 |
| 存储 | 独立表 `random_links`，一个相册最多一条链接 |
| 响应 | `307 Temporary Redirect` 到图片真实直链；接口自身 `Cache-Control: no-store` |
| 版本 | 默认 WebP；`?format=original` 跳原图；图片无 WebP 时回退原图 |
| 参与范围 | 相册内所有 `state = active` 的图片，不看相册或图片的 `is_public` |
| 抽图 | 候选池缓存 + 主动失效；本轮只有内存实现，接口为 Redis 预留 |
| 启动密钥 | 不使用。重启、多实例下链接保持有效 |

## 数据

新增迁移 `0005_random_links.sql`（`internal/migrate/postgres` 与 `internal/migrate/sqlite` 各一份），不修改已发布迁移，不依赖 `AutoMigrate`。

- `users` 新增 `public_id TEXT NULL` 与唯一索引。迁移不回填：新用户注册时生成；老用户在首次创建随机链接时补齐。生成用 `crypto/rand`，撞唯一索引时重试，最多 5 次。
- 新表 `random_links`：

| 列 | 类型 / 约束 | 说明 |
| --- | --- | --- |
| `id` | 主键 | |
| `user_id` | 非空，索引 | 所有者 |
| `album_id` | 非空，唯一 | 一个相册一条链接 |
| `token` | 非空，唯一 | 24 位 base62 |
| `enabled` | 非空，默认 true | 停用后返回 404，保留 token |
| `created_at` / `updated_at` | | |

删除相册时在同一事务内删除其链接。将来支持多相册时，用新迁移增加 `random_link_albums` 关联表并迁移 `album_id`。

## 接口

### 公开接口

`GET|HEAD /random/:uid/:token[?format=original]`

- 成功：`307`，`Location` 为直链（由存储 `base_url` 与对象 Key 实时拼接，URL 不入库）。
- 以下情况返回完全相同的 `404`（原生外壳，`code` 10001），不区分原因：`uid` 不存在、`token` 不存在、二者不属于同一用户、链接已停用、用户被禁用、相册无可用图片。
- `format` 取值只接受空或 `original`，其他值返回 `400`（1xxxx）。
- `token` 比较使用常量时间比较。
- 不返回图片元数据，不返回 EXIF。
- 复用现有按 IP 的 `rateLimit` 中间件；如其阈值是按登录接口设定的，则为本路由单独配置更宽松的阈值，具体数值在实现计划中依据现有实现确定。
- 日志只记录路由模板，`uid` 与 `token` 不进日志。

### 管理接口（需登录，外壳 `{"code","message","data"}`）

| 路由 | 作用 |
| --- | --- |
| `GET /api/albums/:id/random-link` | 查看；不存在时 `data` 为 `null` |
| `PUT /api/albums/:id/random-link` | 创建或更新，body `{"enabled": boolean}`；首次调用生成 token |
| `POST /api/albums/:id/random-link/reset` | 生成新 token，旧链接立即失效 |
| `DELETE /api/albums/:id/random-link` | 删除 |

`data`：`{"enabled": boolean, "path": "/random/<uid>/<token>", "created_at": string}`。后端没有站点对外地址配置，因此只返回 `path`，前端用 `window.location.origin` 拼出完整地址。

只有相册所有者可操作；访问他人相册返回与相册不存在相同的错误。

## 分层与组件

遵循 `http → service → repo`，service 只依赖接口。

- `internal/model/random_link.go`：`RandomLink` 模型。
- `internal/repo/random_link.go`：链接的增删改查；按 `(public_id, token)` 解析出链接、所有者启用状态；按相册列出候选图。
- `internal/service/random_link.go`：`RandomLinkService`，含管理操作与 `Pick(ctx, uid, token, format) (url string, err error)`。
- `internal/service` 中定义接口：

```go
// RandomPool caches the redirect candidates of one album.
type RandomPool interface {
	Get(ctx context.Context, albumID uint64) ([]RandomCandidate, bool, error)
	Set(ctx context.Context, albumID uint64, items []RandomCandidate, ttl time.Duration) error
	Invalidate(ctx context.Context, albumID uint64) error
}
```

`RandomCandidate` 只含拼直链所需字段：`StorageID`、`Path`、`Ext`、`HasWebP`、`HasOriginal`。

- `internal/randompool/memory.go`：内存实现，`sync.RWMutex` 保护的 map，带过期时间。Redis 实现将来放在同包另一文件，本轮不引入依赖。
- `internal/http/native/random.go`：公开路由与管理路由的 handler。
- `internal/cli/serve.go`：装配。

## 请求流程

1. handler 校验 `format`，调用 `Pick`。
2. service 用 `(uid, token)` 查链接；校验启用状态与用户状态。此查询每次都走数据库（主键级索引查询），保证停用、重置、禁用用户立即生效。
3. `pool.Get(albumID)`；未命中则由 repo 查出候选图并 `Set`，TTL 60 秒。
4. 候选为空返回 `ErrNotFound`。
5. 用 `math/rand/v2` 随机取一项（选图不是安全场景），按 `format` 与 `HasWebP` 决定对象 Key，查存储 `base_url` 拼直链。
6. handler 返回 `307`。

候选池上限 5000 条；相册图片数超过上限时，repo 随机采样 5000 条装池，池过期后重新采样。

## 失效

以下操作成功后调用 `pool.Invalidate(albumID)`（涉及两个相册时都失效）：上传到相册、`SetAlbum` 移入或移出、进回收站、恢复、彻底删除、管理员删除图片、删除相册。

单实例下主动失效保证进回收站的图立即不再被抽中。多实例且无共享缓存时，其他实例最多延迟 60 秒；此时跳转目标已是 404，不泄露内容。缓存读写失败不影响请求：`Get` 出错按未命中处理并记录日志，`Set`/`Invalidate` 出错只记录日志。

## 错误处理

- 业务错误使用现有哨兵值（`ErrNotFound`、`ErrForbidden` 等），handler 统一映射错误码；错误用 `%w` 包装。
- `public_id` 或 `token` 生成连续冲突 5 次返回内部错误（5xxxx）。

## 前端（web-vben）

- `src/api/albums.ts` 增加四个管理接口的封装，类型由 `docs/openapi.yaml` 生成。
- `AlbumDetailView.vue` 增加“随机图片链接”区块：启用开关、完整链接与复制按钮、“加 `?format=original` 获取原图”的说明、重置按钮（二次确认，提示旧链接失效）、删除按钮。
- 首次启用时提示：相册内所有图片（含私有图片）都可能通过该链接被匿名访问。
- 文案加入 `locales/messages/{zh-CN,en-US}/albums.json`。

## 测试

- repo：链接增删改查、唯一约束、`(public_id, token)` 解析、候选查询排除 `trash`/`pending`、采样上限；PostgreSQL 与 SQLite 都跑。
- 迁移：`0005` 在两种数据库上可应用，旧数据保留。
- service：`Pick` 各 404 分支、WebP 回退、`format=original`、池命中与未命中、缓存出错降级、各失效调用点。
- `randompool`：过期、失效、并发（`-race`）。
- http：`307` 与 `Location`、`no-store`、统一 404、`HEAD`、非法 `format`、管理接口鉴权与越权。
- 前端：API 封装与区块组件的 vitest 测试（启用、复制、重置确认、删除）。
- 目标覆盖率：`service/random_link` 与 `randompool` ≥ 80%。
