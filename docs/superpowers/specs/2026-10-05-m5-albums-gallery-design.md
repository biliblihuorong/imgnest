# M5 相册与画廊 设计补充（2026-10-05）

计划：[M5 plan](../plans/2026-10-05-m5-albums-gallery.md)。蓝空迁移（import-lsky）按用户已确认的决定「对 MySQL/PostgreSQL 源库的支持延后，v1 先不做」继续延后，不在本轮。

## 范围

- **相册原生 CRUD**：`GET/POST /api/albums`、`PATCH/DELETE /api/albums/{id}`（本人；M4 的 AlbumService.List/FindOwned/Delete 复用扩展）。字段：name（必填 ≤100 rune）、intro（≤500）、is_public、cover_image_id（0=无封面；必须是本人图片）。删除相册图片保留（album_id 置 0，M4 已实现）。
- **图片 ↔ 相册**：`POST /api/images/batch` 新增 `action:"album"` + `album_id`（0=移出相册），逐项 207 与既有 batch 一致；`GET /api/images` 增加 `album_id` 过滤（缺省=全部，与 v1 的「0=未归类」quirk 不同——native 语义 album_id 参数缺省不过滤，显式 0=未归类）。上传已支持 album_id（M2）。
- **公开画廊**：`GET /api/gallery`（公开分页：仅 `is_public=true`、active、`gallery_enabled=true`；否则 403/20003 或空——采用：开关关闭时 200 + 空页，避免探测）。项含 key/名称/宽高/云缩略图链接/上传者用户名。`/api/site` 增加 `gallery_enabled`。
- **前端**：相册管理页（列表卡片/新建/编辑/删除/设封面）、上传页相册选择、图片页相册筛选 + 单图移动 + 多选批量（移动/删除/改权限）、画廊页（公开路由 `/gallery`，CSS columns 瀑布流，匿名可看，导航按 gallery_enabled 显示）、AppLayout 匿名态（显示登录入口）。
- **错误码**：沿用现有分段；相册相关失败用 30003?(限流占用)——不新设：名称非法 10001、他人资源 20003/404、引用图片非本人 10001。无新迁移（albums 表与 image_count 列已在 0002）。

## 契约（先于实现冻结；外壳/分页沿用原生）

- `GET /api/albums?page&size&keyword` → `{items:[AlbumView],total,page,size}`；AlbumView `{id,name,intro,is_public,cover_image_id,image_count,cover_thumb_url,created_at,updated_at}`（cover_thumb_url 实时拼接，无封面为空串）。
- `POST /api/albums` body `{name,intro?,is_public?,cover_image_id?}` → 201 AlbumView；`PATCH /api/albums/{id}` 部分更新 → 200；`DELETE` → data:null。
- `GET /api/gallery?page&size` → 与 ImagePage 同形（复用 ImageView，额外 `uploader` 字段）。
- `/api/site` → 增加布尔 `gallery_enabled`（additionalProperties 数组同步 +1）。
- batch 请求体：`{action:"album", ids:[…], album_id:number}`。

## 前端内部契约

- `src/api/albums.ts`（FE-A 独占）：`listAlbums/createAlbum/updateAlbum/deleteAlbum`，类型 `AlbumView/AlbumInput/AlbumPatch/AlbumPage`。
- `src/api/gallery.ts`（FE-B 独占）：`listGallery(params): Promise<ImagePage & {uploader?}>`——直接复用 `@/api/images` 的 `ImagePage` 类型别名 + uploader 可选扩展。
- `stores/site` 增 `galleryEnabled:boolean`（默认 false，ensureLoaded 一并加载）——Phase 0 由父代理加好，两 worker 只读。
- 路由 `/albums`（登录区）、`/gallery`（公开，PUBLIC_PATHS + 独立公开布局）——Phase 0 由父代理加 stub，worker 只换视图。

## 决定与限制

- 画廊不显示 EXIF/GPS/origin 信息，只给云缩略图与 key；上传者仅用户名。
- 相册公开/私有（is_public）本轮只存不消费（画廊按图片 is_public 过滤，「只展示公开相册」的画廊模式留 M6 打磨）。
- image_count 由计数查询实时得出（M4 repo 已按 count join 实现），不维护冗余计数。
- 图片网格多选批量：移动/删除/改权限三项，复用既有 batch 接口；不做跨页选择。
