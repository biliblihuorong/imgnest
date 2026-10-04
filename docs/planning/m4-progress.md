# M4 实施与验收记录（进行中）

计划：[M4 lsky compat & admin](../superpowers/plans/2026-10-04-m4-lsky-admin.md)、设计：[M4 design](../superpowers/specs/2026-10-04-m4-lsky-admin-design.md)。基线 main `e4ed75a`（M3 已合入）；分支 `feat/m4-lsky-admin`。契约来源：`.claude/skills/lsky-api-compat/`。

**状态：Task 1–6 已完成并提交；Task 7（整合冒烟+审查）与 Task 8（文档收尾）待做。** 本页为进度检查点，续做从这里开始。

## 已完成

| 任务 | 结果 | 提交 |
| --- | --- | --- |
| 1 计划/分支 | 计划+设计落盘，分支建立 | `dc5f6ee` |
| 3 管理后台骨架 | /admin 路由+adminGuard（未登录跳转/非admin 403/restore 兜底）+AdminLayout+api/admin.ts 契约冻结+6 stub+403 页 | `dd0fb52` |
| 2 蓝空 v1 兼容层 | 9 路由字段级对齐 skill 契约；游客上传与登录共用 publish 流水线（user_id=0、限流/配额同路径）；0003 迁移（api_enabled/site_name/guest_group_id/registered_ip）；23 黄金 JSON+双库契约测试；修复 4 个真 bug（html 转义、PG 相册列重复 500、strategy_id 二次 preflight、ImageView.Path json tag） | `d0a9bd4` |
| 4 管理 API | /api/admin 21 handler：admin 守卫（role≠admin 403）、用户启停同事务吊销 Token、组/存储/规则 CRUD+引用检查（30008/30009）、存储 config AES-GCM 加密落库且任何响应不回显、测试连接复用 M2 探测、模板 preview、settings 八字段、全站图片/回收站清空；openapi /api/admin 段 14 路径；site_name 读 settings；register 写 registered_ip | 本批 |
| 5/6 管理页面 | 6 页全实现：Users（分页/搜索/启停回滚/改组）、Groups（CRUD+30008 文案+MB→字节）、Settings（八字段+校验）、Storages（S3 凭证仅提交不回显+测试连接三项+30009）、Policies（四区表单+模板预览）、ImagesAdmin（过滤+移入回收站+清空强确认） | 本批 |

## 验证证据（容器内实测）

- Go 全量 `go test ./internal/... -count=1`：15 包全 ok（双库；PG/MinIO 实跑）；golangci-lint 0 issues。
- 前端全链：vitest **28 文件 225 用例全过**、vue-tsc 双配置、eslint 全绿、（M4 尚未跑过 pnpm build，Task 7 补）。

## Task 7/8 续做清单

1. `make release`（fe-build+嵌入构建）+ 真实二进制冒烟：curl 复刻 picgo-plugin-lankong（Bearer+multipart+Accept: application/json）逐字段断言 v1 响应；游客上传开/关；admin API 走查；浏览器一次（admin 登录→6 页）。
2. 独立审查 worker：Review Focus 五项（v1 契约逐字段、游客不绕过、admin 授权与密钥不回显、列表 quirk、上传复用）。
3. 文档：development.md（v1/admin/PicGo 手工联调步骤）、README、versions 执行补充、本页完善；推送合并。
4. 已知待办：FE admin 类型仍手写（可在 openapi 落地后 gen:api 切换派生）；purge-all 部分失败时前端展示 data.purged 需 client 支持（当前 ApiError 不带 data，可接受降级）；`permission` 缺省=私有（与部分 Lsky 版本默认公开不同，openapi 已注明）；`date` 为 UTC 非服务器时区。

## 语义决定（续做勿翻案）

- 组删除：is_default/is_guest 拒删（403）；有成员 30008；被 settings 引用 30009。存储被规则引用 30009；规则被图片/组默认引用 30009。purge-all 失败不中断，全成 200，部分失败 502/50002+data.purged。禁用用户与 Token 吊销同事务、禁自己 403。PATCH 组不接受 is_default/is_guest（仅创建时）。
- 游客组解析：settings guest_group_id > is_guest=1 组 > 无则 401。游客行 user_id=0，锚定行不计费。
