# M4 蓝空 v1 兼容 + 管理后台 设计补充（2026-10-04）

计划：[M4 plan](../plans/2026-10-04-m4-lsky-admin.md)。契约唯一来源：`.claude/skills/lsky-api-compat/`（SKILL.md + references/contract.md）；冲突时以 spec 与 skill 为准。

## 范围

- `/api/v1/*` 全量蓝空 v1 兼容层（tokens、strategies、upload、images、images/{key} 删除、albums、profile），错误外壳/HTTP 状态/字段单位逐字对齐 skill 契约；黄金 JSON 契约测试落 `internal/http/lsky/testdata/`。
- 游客上传（v1 upload 无 Token 分支）：`guest_upload_enabled` + `guest_group_id`（或 `is_guest=1` 组）+ 组 `upload_per_min` 每 IP 限流 + 游客容量按 `user_id=0` 图片字节和对组容量校验。
- 原生管理后台 API `/api/admin/*` 与 Vue 管理页面（用户/组/存储/规则/设置/全站图片与回收站）。
- 0003 迁移：settings 增加 `api_enabled`（默认 true）、`site_name`（默认 "ImgNest"）、`guest_group_id`（默认 0）；users 增加 `registered_ip`。两库同步；0001/0002 不改。
- `tasks/backfill`（存量补处理）与画廊/相册原生 CRUD/封面不在 M4（M5）；`tasks/backfill` 的管理入口延后。
- PicGo 联调关口：本轮以 `picgo-plugin-lankong` 同形态 curl 冒烟 + 黄金 JSON 断言代替（无桌面客户端环境）；真实 PicGo 手工验证留给用户，步骤写进 development.md。

## 关键实现决定

- **v1 handler 独立包 `internal/http/lsky`**，不复用 native 外壳；service 层复用（同一 UploadService/ImageService/TokenService/UserService），不复制业务。
- **游客 subject**：`TokenSubject` 保持不透明；service 层新增显式游客分支（guest 走 guest group 的 Preflight/配额/规则，不查 users 行）。携带伪造/失效 `Authorization` 的 upload 一律 401，绝不静默降级游客。
- **ImageView 增加 `Path`**（不含扩展名的存储路径，如 `2026/10/04/66ff…`）：v1 `name` = Path 基名 + `.` + ext，`pathname` = Path + `.` + ext。native DTO 新增该字段不违反隐私约束（URL 本就公开）。
- **profile.registered_ip**：注册时写入 users.registered_ip（0003 新列），历史用户为空串。
- **v1 时间**：`date` 为 UTC 的 `Y-m-d H:i:s`；`human_date` 为中文相对时间（刚刚/N 分钟前/N 小时前/N 天前/日期）。
- **albums 最小面**：repo/service 提供 List（含 image_count）/Delete（images.album_id 置 0，图片保留）；原生 `/api/albums` CRUD 仍留 M5。
- **admin 中间件**：native Identity 已含 role；`/api/admin/*` 要求 `role=admin`，否则 403/20003。
- **新错误码**：30008 组仍有成员（409）、30009 存储被规则引用（409）。其余沿用 M1/M2 分段。
- **Site 服务升级**：`site_name` 改读 settings（0003 键），缺省回落 "ImgNest"；管理设置可改。
- 前端管理页 token/权限沿用 M3 机制；`/admin/*` 路由要求 `user.role === 'admin'`，非 admin 显示 403 页。admin API 类型先手写（openapi 由后端 worker 顺序补齐后再 gen:api 切换派生）。

## /api/admin/* 契约（契约先行；外壳/错误码沿用原生）

| 接口 | 方法/说明 |
| --- | --- |
| `/api/admin/users` | GET 分页（page/size/keyword 搜 username/email）；PATCH `/users/{id}` body `{status?, group_id?}`，返回 AdminUserView |
| `/api/admin/groups` | GET 列表（含成员数）；POST 创建；PATCH/DELETE `/{id}`；删除时仍有成员→30008 |
| `/api/admin/storages` | GET 列表；POST 创建（S3 config 密钥 AES-GCM 落库，响应不含密钥）；PATCH/DELETE `/{id}`；被规则引用→30009；POST `/{id}/test` → `{ok, checks:{put,copy,delete}}`（复用 M2 连接测试） |
| `/api/admin/policies` | GET 列表；POST 创建；PATCH/DELETE `/{id}`；POST `/preview` body `{path_tpl,name_tpl}` → `{sample, error}`（pathtpl 渲染样例） |
| `/api/admin/settings` | GET/PUT，字段：site_name、registration_enabled、guest_upload_enabled、gallery_enabled、trash_days、api_enabled、guest_group_id、default_group_id |
| `/api/admin/images` | GET 全站分页（page/size/user_id/keyword）；DELETE `/{id}` 移入回收站（owner 维度） |
| `/api/admin/trash/purge-all` | POST 清空全站回收站 → `{purged: N}` |

AdminUserView `{id,username,email,role,status,group_id,used_bytes,created_at}`；GroupView `{id,name,is_default,is_guest,capacity_bytes,max_file_bytes,allowed_exts,upload_per_min,default_policy_id,policy_ids,user_count}`；PolicyView 为规则全字段 + storage_id + enabled；StorageView `{id,name,driver,base_url,enabled}`（config 永不回显）。
