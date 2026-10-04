# M4 实施与验收记录

计划：[M4 lsky compat & admin](../superpowers/plans/2026-10-04-m4-lsky-admin.md)、设计：[M4 design](../superpowers/specs/2026-10-04-m4-lsky-admin-design.md)。基线 main `e4ed75a`（M3 已合入）；分支 `feat/m4-lsky-admin`，完成后合入 main。契约唯一来源：`.claude/skills/lsky-api-compat/`。

用户于 2026-10-04/05 确认：继续 M4 并行执行、推送远程、合入 main；夜间自主完成 M4 收尾后关机（无人在场，故真实 PicGo 桌面客户端联调仍留作用户手工步骤，本轮以 curl 等价形态冒烟为关口证据）。

## 交付范围

| 任务 | 实际结果 |
| --- | --- |
| 1 计划/分支 | 计划+设计落盘，分支建立 |
| 2 蓝空 v1 兼容层 | 9 路由字段级对齐：tokens（form/JSON 双形态、3/min 限流）、strategies、upload（游客分支）、images（Laravel 分页器、order/permission/album_id=0 未归类 quirk/keyword）、images/{key} 删除、albums、profile；0003 迁移（api_enabled/site_name/guest_group_id + users.registered_ip）；游客上传与登录共用 publish 管线（user_id=0 锚定行、组限流、配额同路径） |
| 3 管理后台骨架 | /admin 路由+adminGuard（restore 兜底）+AdminLayout+api/admin.ts 契约冻结+403 页 |
| 4 管理 API | /api/admin 21 路由：admin 守卫、用户启停同事务吊销 Token、组/存储/规则 CRUD+引用检查（30008/30009）、S3 config AES-GCM 加密落库且响应永不回显、测试连接复用 M2 探测、模板 preview、settings 八字段、全站图片与回收站清空；site_name 读 settings；register 写 registered_ip |
| 5/6 管理页面 | Users（分页/搜索/启停回滚/改组）、Groups（CRUD+30008 文案+MB↔字节）、Settings（八字段校验）、Storages（S3 凭证仅提交不回显+测试连接三项+30009）、Policies（四区表单+模板预览）、ImagesAdmin（过滤+移入回收站+清空强确认） |
| 7 整合关口 | make release 嵌入构建；真实二进制冒烟（PicGo 形态 v1 全链、游客开/关闭环、admin API、权限隔离、日志无 token）；浏览器走查（登录/上传/图片/Token/admin 六页全部渲染，DOM 快照留档）；独立审查（无 P1，P2 已修） |
| 8 文档收尾 | 本记录、development.md v1/管理后台/PicGo 联调说明、README、versions 执行补充、计划状态更新 |

## 验证证据

全部容器内执行；PG/MinIO 实跑，无 skip 代替。

- Go 全量 `go test ./internal/... -count=1`：15 包全 ok（双库）；golangci-lint 0 issues；`make release` 真实嵌入二进制构建通过。
- 前端：vitest **28 文件 225 用例全过**（含 25 个黄金 JSON 契约对、守卫/页面/组件）、vue-tsc 双配置、eslint 全绿。
- 真实二进制冒烟（SQLite+本机存储，curl 复刻 picgo-plugin-lankong 请求形态）：v1 tokens 签发/错误凭证 HTTP 200+status:false/strategies/upload 逐字段（pathname、size KB 浮点、md5/sha1、links.html `&lt;img src="…" `、thumbnail `_thumbs`、origin_url/webp_url、webp_url 因 skip_if_larger 为空符合预期）/images 分页/profile（KB 容量、image_num）/albums 空分页/album_id=1 业务失败/坏 Token 401 不降级游客/无 Token 401（游客关闭）→ admin 建游客组绑默认规则+开开关 → 匿名上传 `status:true` → 关开关恢复 401 → v1 删除旧 URL 404 → admin purge-all `{purged:N}` → 服务端日志无任何 token。
- 浏览器走查：登录页（注册入口按 register_enabled=false 正确隐藏）、上传页（拖拽区/限额文案/策略自动选中/公开私有开关）、图片页（双页签+空态）、Token 页（创建表单+api 类型说明+表格）、/admin/users|groups|storages|policies|settings|images 六页（AdminLayout 侧边导航+页面标题全部渲染）。
- 独立审查：五项 Review Focus 全过（黄金契约严格性、游客管线同源与锚定行、admin 21 路由守卫矩阵与 config 不回显、FE/后端 21 函数逐一对照零不一致、列表 quirk 与 LIKE 转义）；P2（v1 删除路由缺黄金测试）已补 `TestV1ImageDeleteContract`（成功/幂等重删/不存在 key/未登录，双库绿）。

## 修复与已知限制

- 续做 worker 修复 4 个真 bug：links.html 引号过度转义（`\u0026` 系 wire 编码、解码后与蓝空一致，黄金按值锁定）；PG 相册列表列重复 500；二次 preflight 丢 strategy_id；ImageView.Path 暴露进 native DTO（改 `json:"-"`）。
- 独立审查 P2：v1 删除路由补黄金契约测试（已修）。P3 记录：上传 429 分支无专项测试；v1 上传每次 3 遍 Preflight（冗余但正确）；PatchUser 双字段两个事务（部分成功窗口）；GroupsView 非整 MB 容量编辑往返截断；StoragesView「换凭证需重建」文案与后端能力不符（PATCH 已支持 config 轮换）；限流器单实例内存态（多副本部署前需外置）。
- 游客组未绑 default_policy_id 时，v1 游客上传报笼统的「上传失败，请稍后再试」（组无可用规则）；管理 UI 建组强制必选默认规则，真实管理员不会遇到。
- IAB webview 自动化怪癖：合成提交实际成功（token 签发、localStorage 写入）但 SPA 内 `router.push` 未生效，且伴随一次「网络错误」告警；整页加载全部正常，vitest 守卫/跳转 7 用例全绿。判定为自动化环境特有，待用户在真实浏览器做一次登录确认。
- v1 `date` 为 UTC（非服务器时区）；`permission` 缺省=私有（部分 Lsky 版本默认公开，openapi 已注明）；FE admin 类型手写（openapi /api/admin 段已落地，后续可 gen:api 切换派生）。
- `tasks/backfill` 存量补处理、相册原生 CRUD/封面、画廊 → M5；多副本限流、arm64 镜像 → M5/M6。

## 提交记录

`dc5f6ee` docs 计划 → `dd0fb52` feat 管理后台骨架 → `d0a9bd4` feat 蓝空 v1 兼容层 → `d7c9dda` feat 管理 API → `594d69b` feat 用户/组/设置页面 → `25b1529` feat 存储/规则/图片页面 → `67ac8f8` docs 进度检查点 → `test` v1 删除契约 → `docs` 本记录。已合入 main 并推送（无 tag）。
