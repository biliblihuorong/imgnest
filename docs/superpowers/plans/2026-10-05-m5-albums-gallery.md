# ImgNest M5 Albums & Gallery Implementation Plan

> **For agentic workers:** 沿用 M3/M4 模式：契约先行、RED→GREEN TDD、并行分工（workers 不执行 Git 写操作）、父代理整合审查。步骤 checkbox 为验收依据，实际进度以 `docs/planning/m5-progress.md` 为准。

**Goal:** 相册原生 CRUD + 图片按相册筛选/移动/多选批量 + 公开画廊（匿名可访问、开关控制）+ 上传选相册；全链测试绿、真实二进制冒烟、独立审查通过。

**Architecture:** 复用 M4 AlbumService 骨架扩展 Create/Update/封面；画廊是新只读端点（仅 is_public+active）；batch 增加 album 动作；SiteView 增 gallery_enabled。前端两个页面域 + 网格多选，路由 stub 由父代理先行。

**Tech Stack:** 无新依赖；无新迁移（0002 已有 albums 表）。

**Spec:** [spec.md](../../spec.md) §2（P1 相册/画廊）、[M5设计补充](../specs/2026-10-05-m5-albums-gallery-design.md)。基线 main `8827b1d`。分支 `feat/m5-albums-gallery`。**import-lsky 按用户决定（v1 先不做 MySQL/PG 源库）延后，不在本轮。**

## Global Constraints

- 分层单向；service 只依赖接口；业务函数 context 首参；错误 %w + 哨兵。
- 画廊响应绝不含 EXIF/GPS/IP；匿名仅见 is_public 且 gallery_enabled 开启的 active 图片。
- 相册操作全部 owner 校验；cover_image_id 必须是本人 active 图片；删除相册不删图（album_id 置 0）。
- native images 列表 album_id 参数：缺省=不过滤，显式 0=未归类（与 v1 quirk 区分并在 openapi 写明）。
- 前端禁 v-html；画廊瀑布流纯 CSS columns；AppLayout 匿名态不崩溃（显示登录按钮）。
- workers 文件边界见下表；openapi 段归 BE worker；禁改他人文件；禁 Git 写命令。

## Review Focus

1. 画廊越权：私有图/回收站图/关闭开关时的响应；匿名访问路径。
2. 相册 owner 边界：CRUD/封面/移动对他人资源的拒绝；删除相册后图片 album_id 置 0 且计数正确。
3. batch album 动作：逐项结果语义与既有 delete/permission 一致；album_id 归属校验。
4. SiteView 增字段不破坏 M3 契约测试与前端 site store。
5. AppLayout 匿名渲染与 /gallery 公开路由守卫（PUBLIC_PATHS）。

## 文件边界

| 阶段 | 工作区 | 文件 |
| --- | --- | --- |
| 父代理 Phase 0 | docs/superpowers、web/src/router/index.ts、web/src/views/{AlbumsView,GalleryView}.vue(stub)、web/src/stores/site.ts(+test)、分支 | 计划、路由 stub、site store galleryEnabled |
| BE worker | internal/service/album.go+test、internal/service/image*.go（batch/gallery）、internal/repo/album.go+test、internal/repo/image*.go、internal/http/native/album.go(新)+image.go(最小扩展)+test、internal/service/site.go+test、internal/repo/settings.go(若需)、internal/http/router.go(注册)、internal/cli/serve.go(接线)、docs/openapi.yaml | 相册 CRUD/封面、batch album、gallery、SiteView 扩展 |
| FE-A worker | web/src/api/albums.ts(+test)、web/src/views/AlbumsView.vue(+test 替换 stub)、web/src/components/albums/**、web/src/views/UploadView.vue(+test 加相册选择)、web/src/views/ImagesView.vue(+test 相册筛选/移动/多选批量)、web/src/components/images/** | 相册页 + 上传/图片页相册能力 |
| FE-B worker | web/src/api/gallery.ts(+test)、web/src/views/GalleryView.vue(+test 替换 stub)、web/src/components/gallery/**、web/src/components/layout/AppLayout.vue(+test 匿名态/画廊入口) | 画廊页 + 布局匿名态 |
| 父代理整合 | 全量验证、冒烟、审查、docs、合并 |  |

## Task 1 计划与 Phase 0（父代理）

- [ ] 设计/计划落盘、建分支提交；router 加 /albums（登录区）与 /gallery（公开，PUBLIC_PATHS）stub、site store 增 galleryEnabled（含测试更新）；提交 `docs: add M5 albums gallery plan` + `feat: stub album and gallery routes`。

## Task 2 后端相册/画廊/批量（BE worker，与 Task 3/4 并行）

**Files:** 见上表。要点：
- [ ] RED：相册 CRUD owner 边界（他人 404/20003）、封面非本人图片拒绝、SiteView 新字段、gallery 开关关闭/私有图/回收站排除、batch album 逐项、images?album_id 过滤语义。
- [ ] AlbumService.Create/Update（name≤100 rune、intro≤500）；FindOwned 复用；Delete 置空图片 album_id（已有）并确认计数。
- [ ] Gallery：只读、分页、仅 public+active；uploader 取用户名；开关关闭返回空页（不报错）。
- [ ] openapi：/api/albums、/api/gallery、batch action 扩展、SiteView 扩展、GET /api/images 增 album_id 参数。
- [ ] 容器内 `go test ./internal/... -count=1` 全绿 + lint 0。

## Task 3 相册页与图片页相册能力（FE-A worker，与 Task 2/4 并行）

- [ ] api/albums.ts 按设计契约（mock 测试）；AlbumsView：卡片网格（封面缩略图/名称/数量/公开标记）、新建/编辑弹窗（name/intro/is_public/封面选择从我的图片选取——简化：输入图片 ID 或从最近图片选择器）、删除确认（提示图片保留）。
- [ ] UploadView：相册下拉（listAlbums，可空=不入相册），提交带 album_id；既有测试更新。
- [ ] ImagesView：相册筛选下拉（全部/未归类/各相册）、卡片菜单「移动到相册」、多选（checkbox）批量条（移动/删除/改权限，调 batch）；既有测试更新。
- [ ] 容器内 vitest/typecheck/lint 自己文件清零。

## Task 4 画廊页与匿名布局（FE-B worker，与 Task 2/3 并行）

- [ ] api/gallery.ts（复用 ImagePage 类型）；GalleryView：CSS columns 瀑布流、NImage 懒加载、分页、空态/开关关闭文案、点击放大（NImage preview）。
- [ ] AppLayout：galleryEnabled 时显示「画廊」菜单项（匿名+登录都显示画廊；匿名时右侧显示「登录」按钮而非用户下拉——匿名访问 /gallery 直接整页可用，不跳登录）。
- [ ] 容器内 vitest/typecheck/lint 自己文件清零。

## Task 5 整合与关口（父代理）

- [ ] 全量：Go 套件、FE 四链、make release；真实二进制冒烟：建相册→传图入相册→列表筛选→移动→封面→画廊开/关匿名访问；浏览器走查画廊匿名态与相册页。
- [ ] 独立审查 worker 复查 Review Focus；P1/P2 修复后全绿。

## Task 6 文档与收尾（父代理）

- [ ] development.md（相册/画廊）、README、m5-progress、计划状态；提交并合 main 推送（不 tag）。

## 当前交接

完成后 v1 功能面仅剩 import-lsky（已延后）与 M6 打磨（生产镜像/CI/存量补处理/多架构发布）。
