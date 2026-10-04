# ImgNest M4 Lsky Compat & Admin Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development 或 executing-plans，逐任务推进。沿用 M2/M3 的契约先行、RED→GREEN TDD、并行分工与末尾独立审查；workers 不执行 Git 写操作，提交由父代理统一完成。

**Goal:** `/api/v1/*` 与蓝空 v1 字段级兼容（黄金 JSON 契约测试锁定），游客上传可用；原生管理 API 与 Vue 管理后台交付；真实二进制以 PicGo 请求形态冒烟通过。

**Architecture:** v1 兼容层在新包 `internal/http/lsky`，只做协议映射，业务全部复用现有 service（UploadService/ImageService/TokenService/UserService/新增 AlbumService）；游客是 service 层显式分支。admin 是 native 上的 role 守卫子树。0003 迁移只增不改。

**Tech Stack:** 无新 Go/前端依赖；测试沿用 testify + 容器内双库/MinIO；前端沿用 M3 脚手架。libvips/版本矩阵不动。

**Spec:** [spec.md](../../spec.md) §2/§4/§7/§8.2、[lsky-api-compat skill](../../../.claude/skills/lsky-api-compat/SKILL.md)（契约唯一来源）、[M4设计补充](../specs/2026-10-04-m4-lsky-admin-design.md)。基线 main `e4ed75a`（M3 已合入）。分支 `feat/m4-lsky-admin`。

**Status:** 计划已获用户确认并行执行（2026-10-04，"继续后续任务"）。实际完成情况以 `docs/planning/m4-progress.md` 为准。

## Global Constraints

- `/api/v1` 字段/类型/单位逐字对齐 skill：**只多不少**；size/capacity/used_capacity 为 KB 浮点；业务失败 HTTP 200 + status:false；仅 401/403(API禁用)/429 使用语义 HTTP 码。
- v1 一律不返回 EXIF/GPS；images `album_id` 缺省或 0 = 「未归类」是蓝空 quirk，保留。
- 两套 API 共用同一 service；禁止为 v1 复制第二套上传/配额逻辑；游客路径不绕过配额预约与补偿。
- 0003 迁移只新增（settings 三键 + users.registered_ip）；0001/0002 哈希不变；无 AutoMigrate。
- 密钥/Token/GPS 不进日志；admin storages 配置永不回显；注册 IP 仅本人 profile 可见。
- admin 删除组/存储必须先做引用检查（30008/30009），不做级联删除。
- workers 文件边界见下表；openapi.yaml 按阶段独占（阶段1归 BE-v1，阶段2归 BE-admin）；不改他人目录、不动锁文件、不执行 Git 写命令。

## Review Focus

1. v1 契约逐字段：黄金 JSON 断言必须覆盖 tokens/strategies/upload/images/albums/profile 成功与失败形态，字段缺失/单位错误视为 P1。
2. 游客上传不能成为绕过：配额预约、路径唯一、补偿清理与登录用户同路径；`Authorization` 无效必须 401 而不是降级；`guest_upload_enabled=false` 时无 Token 上传 401。
3. admin 授权：非 admin 访问 /api/admin/* 一律 403/20003； storages config 任何响应/日志不出现密钥；删除组/存储的引用检查在并发下不产生悬挂引用。
4. 图片列表过滤 quirk：album_id=0/缺省=未归类；keyword/permission/order 组合结果稳定；40/页。
5. 上传链路复用：strategy_id→policy 归属校验（本人组启用规则）、permission→is_public、album_id 归属校验，全部走既有 Preflight/Upload。

## 文件边界与并行顺序

| 阶段 | 工作区 | 文件 |
| --- | --- | --- |
| 父代理先行 | docs/superpowers、分支 | 计划文档、建分支 |
| 阶段1 BE-v1 worker | internal/http/lsky/**、internal/service（游客/相册/ImageView.Path）、internal/repo（album、游客配额查询）、internal/migrate 0003、internal/http/router.go、internal/cli/serve.go、docs/openapi.yaml（仅 /api/v1 段）、internal/http/lsky/testdata | 蓝空 v1 全量 + 游客上传 + 黄金契约测试 |
| 阶段1 FE-admin worker | web/src/**（admin 路由/布局/api 模块/类型/守卫/6 页 stub + 403 页） | 管理后台骨架（不碰 openapi/Go） |
| 阶段2 BE-admin worker | internal/http/native/admin*.go、internal/service admin 域、internal/repo、internal/http/router.go、internal/cli/serve.go、docs/openapi.yaml（仅 /api/admin 段）、service/site.go（site_name 读 settings） | admin API 全量 |
| 阶段2 FE-pages-A worker | web/src/views/admin/{UsersView,GroupsView,SettingsView}.vue + 组件/测试 | 用户/组/设置页面 |
| 阶段2 FE-pages-B worker | web/src/views/admin/{StoragesView,PoliciesView,ImagesAdminView}.vue + 组件/测试 | 存储/规则/全站图片页面 |
| 父代理整合 | Makefile、docs、progress、冒烟、审查 | 全量验证、PicGo 形态冒烟、独立审查、提交推送 |

阶段1两路并行；阶段2三路并行（BE-admin 独占 openapi 与 Go；FE 两个 worker 各自独占页面文件，admin api 模块由阶段1 FE worker 冻结）。

## Shared Interfaces

v1 协议映射（BE-v1 冻结；黄金测试锁定）：

- 外壳 `{status,message,data}`；data 空为 `{}`；失败 HTTP 200 + status:false；401 `Unauthenticated.`、403 `管理员未启用 API`（settings api_enabled=false 时全 v1 路由）、429 `Too Many Attempts.`
- upload data 字段见 skill contract（size=KB 浮点、html 转义格式、thumbnail_url 无云缩略图时回退 url、origin_url/webp_url 额外字段）；list 为 Laravel 分页器形状（current_page/data/last_page/per_page/total/url 字段）
- 新 service 面：`AlbumService.List/Delete`（owner 校验、image_count、删除置空 album_id）；游客分支 `GuestUpload`（guest group 解析：settings guest_group_id>0 优先，否则 is_guest=1 组；两处都没有→v1 上传无 Token 直接 401）；`ImageView` 增加 `Path string`

admin 契约：见设计补充表格（AdminUserView/GroupView/PolicyView/StorageView/settings 字段清单）。前端 admin api 模块签名由 FE-admin worker 冻结在 `web/src/api/admin.ts`（或分域文件），阶段2 页面 worker 只消费。

## Task 1: 计划与分支（父代理，已完成于发牌前）

- [x] 设计补充与计划落盘；从 main 建 `feat/m4-lsky-admin`；提交 `docs: add M4 lsky compat and admin plan`。

## Task 2: 蓝空 v1 兼容层（BE-v1 worker，与 Task 3 并行）

**Files:** internal/http/lsky/**（handler+分页器+映射+testdata 黄金 JSON）、internal/service（album、guest、ImageView.Path、UploadInput 复用）、internal/repo（album、游客容量查询）、internal/migrate/{sqlite,postgres}/0003_*.sql、internal/http/router.go、internal/cli/serve.go、docs/openapi.yaml（/api/v1 段）。

- [ ] 0003 先行（RED：迁移测试断言新 settings 键与列；0001/0002 哈希不变）。
- [ ] tokens（3/min/IP 复用限流器）、DELETE tokens、strategies（登录→本人组规则；无 Token→游客组规则或空）、profile（KB 容量、image_num/album_num、registered_ip）。
- [ ] upload：api_enabled 检查；strategy_id/album_id/permission 校验；游客分支（限流=游客组 upload_per_min、容量=user_id=0 已占+预约 ≤ 组容量）；失败映射 status:false 中文消息；成功字段逐字对齐（含 html 转义、thumbnail 回退、origin_url/webp_url）。
- [ ] images 列表（Laravel 分页器、40/页、order/permission/album_id(0=未归类)/keyword(origin_name/pathname LIKE)）、DELETE /images/{key}→回收站、albums 列表（分页+image_count）/删除。
- [ ] 黄金 JSON：testdata 每路由至少成功+代表性失败各一；测试逐字段断言。
- [ ] Run 容器内双库 `go test ./internal/http/... ./internal/service ./internal/repo ./internal/migrate -count=1`；Expected exit0。
- [ ] 提交（父代理）`feat: serve lsky v1 compatible api`。

## Task 3: 管理后台骨架（FE-admin worker，与 Task 2 并行）

**Files:** web/src/router（/admin/* 子路由 + admin 守卫）、web/src/components/admin/AdminLayout.vue（侧边导航）、web/src/api/admin*.ts（契约模块+类型，阶段2 页面只消费）、403 页、6 个页面 stub（Users/Groups/Settings/Storages/Policies/ImagesAdmin）。

- [ ] admin 守卫：未登录跳登录；非 admin（user.role!=='admin'）渲染 403 页；api 模块按设计补充契约冻结签名。
- [ ] 组件测试：守卫三分支、api 模块成功/失败解包；`pnpm vitest run && pnpm typecheck && pnpm lint` exit0。
- [ ] 提交（父代理）`feat: scaffold admin console shell`。

## Task 4: 管理 API（BE-admin worker，与 Task 5/6 并行）

**Files:** internal/http/native/admin*.go、admin 中间件、internal/service admin 域（users/groups/storages/policies/settings/imagesAdmin）、internal/repo 对应查询、router.go、serve.go、openapi（/api/admin 段）、service/site.go（site_name 读 settings）+ register 写 registered_ip。

- [ ] RED 先行：admin 403 守卫、CRUD、引用检查（30008/30009）、storage test 复用、settings 读写、全站 images/trash。双库测试。
- [ ] Run 容器内 `go test ./internal/... -count=1`；Expected exit0。
- [ ] 提交（父代理）`feat: expose admin management api`。

## Task 5: 用户/组/设置页面（FE-pages-A worker，与 Task 4/6 并行）

**Files:** web/src/views/admin/{UsersView,GroupsView,SettingsView}.vue + 组件/测试。

- [ ] Users：分页+keyword、启停、改组；Groups：列表+成员数、CRUD、容量/单文件/扩展名/每分钟上传/默认规则/绑定规则编辑、删除确认（30008 提示）；Settings：八字段表单（含 guest_group_id/default_group_id 下拉用组列表）保存。
- [ ] 测试 mock api：加载/保存/错误/确认流。容器内 `pnpm vitest run && pnpm typecheck && pnpm lint`。
- [ ] 提交（父代理）`feat: build admin user group settings pages`。

## Task 6: 存储/规则/全站图片页面（FE-pages-B worker，与 Task 4/5 并行）

**Files:** web/src/views/admin/{StoragesView,PoliciesView,ImagesAdminView}.vue + 组件/测试。

- [ ] Storages：列表+创建（本机/S3，S3 凭证仅提交不回显）+启用开关+「测试连接」（展示 put/copy/delete 结果）+删除（30009 提示）；Policies：列表+创建/编辑（模板/参数表单）+「预览」样例展示+删除；ImagesAdmin：全站分页+user_id/keyword 过滤+移入回收站；顶部「清空回收站」强确认。
- [ ] 测试 mock api：加载/保存/测试连接/预览/确认流。容器内验证链 exit0。
- [ ] 提交（父代理）`feat: build admin storage policy images pages`。

## Task 7: 整合与关口（父代理）

- [ ] gen:api 重生成，FE admin 类型切 schema 派生（可保留手写兜底）；全量：Go 套件、FE 四链、lint、make release。
- [ ] 真实二进制冒烟：init 后 curl 复刻 picgo-plugin-lankong 请求（Bearer + multipart file + Accept: application/json）断言 v1 响应逐字段；游客上传开/关两态；admin API 与页面 API 走查（浏览器一次）。
- [ ] 独立审查 worker 复查 Review Focus 五项；问题 RED→GREEN 修复后全绿。

## Task 8: 文档与收尾（父代理）

- [ ] development.md（v1/管理端/PicGo 手工联调步骤）、README、versions 执行补充、`docs/planning/m4-progress.md`；分任务 Conventional Commits；推送分支（main 合并待用户指令）。

## 当前交接

完成后进入 M5（相册原生 CRUD/封面、画廊、游客完善、import-lsky 接管）。本轮不宣称蓝空迁移完成。
