# 原生 API v2

`/api/v2` 是 ImgNest 自己的版本化接口,给脚本和上传客户端用。它与[蓝空兼容的 `/api/v1`](./lsky-api)并存:旧的 PicGo / Typora / uPic 配置不用动,新客户端可以逐步迁到 v2。

## 与 v1 的区别

- **单位是字节**:v1 为了兼容蓝空用 KB 浮点,v2 一律字节。
- **外壳是原生格式**:`{"code": 0, "message": "ok", "data": ...}`,`code ≠ 0` 失败,`data.reason` 可能带提示码(错误码语义见 OpenAPI 文件)。
- **稳定承诺**:v2 只会增加字段和路由,不改名、不删除。
- **行为与控制台一致**:每个 v2 路由由对应的 `/api` 路由同一套 handler 和 service 提供,校验、响应与错误码就是 OpenAPI 文档里那份。
- 和 v1 一样,**任何 v2 响应都不含 EXIF / GPS**。

## 认证

`Authorization: Bearer <token>`,令牌在网页「账户与令牌」页创建,明文只在创建时显示一次。管理员的 API 开关(`api_enabled`)同时管 v1 与 v2:关闭时 v2 返回 `403`,`code: 20003`,`data.reason: "api_disabled"`。

## 路由一览

| 方法 | 路径 | 等价于 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/v2/profile` | `GET /api/auth/me` | 当前账户与用量 |
| GET | `/api/v2/policies` | `GET /api/policies` | 可用的上传规则 |
| POST | `/api/v2/upload` | `POST /api/upload` | multipart `file`,可选 `policy_id`、`album_id`、`is_public` |
| GET | `/api/v2/images` | `GET /api/images` | 分页、过滤与[统一搜索](./search)(`qv=1&q=...`) |
| GET | `/api/v2/images/{id}` | `GET /api/images/{id}` | |
| PATCH | `/api/v2/images/{id}` | `PATCH /api/images/{id}` | `is_public` 和/或 `name`(显示名;链接不变) |
| DELETE | `/api/v2/images/{id}` | `DELETE /api/images/{id}` | 移入回收站 |
| POST | `/api/v2/images/batch` | `POST /api/images/batch` | `delete` / `permission` / `album`(移动) |
| GET | `/api/v2/albums/{id}/images` | `GET /api/albums/{id}/images` | 相册内搜索 |
| GET | `/api/v2/trash` | `GET /api/trash` | 回收站列表 |
| POST | `/api/v2/trash/restore` | `POST /api/trash/restore` | 从回收站恢复 |
| GET | `/api/v2/albums` | `GET /api/albums` | |
| POST | `/api/v2/albums` | `POST /api/albums` | |
| PATCH | `/api/v2/albums/{id}` | `PATCH /api/albums/{id}` | |
| DELETE | `/api/v2/albums/{id}` | `DELETE /api/albums/{id}` | 图片保留,只是退出相册 |

## 示例

```bash
# 上传
curl -H "Authorization: Bearer $TOKEN" -F file=@photo.jpg https://img.example.com/api/v2/upload

# 搜索自己的图片:2026 年 10 月以后、jpg、按大小降序
curl -H "Authorization: Bearer $TOKEN" \
  "https://img.example.com/api/v2/images?qv=1&q=format%3Ajpg%20after%3A2026-10-01%20sort%3Asize-desc&tz=Asia%2FShanghai&page=1&size=20"
```

完整字段定义在仓库的 `docs/openapi.yaml`,前端类型也是从它生成的。
