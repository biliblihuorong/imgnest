# 前端真实 API → Go HTTP → SQLite / 本地存储

这组独立测试直接导入 `web-vben/src/api`，请求现有 Go 二进制提供的真实本地 TCP 服务。它不使用 `e2e/fixtures/server.mjs`，没有 mock API 响应、路由、service 或数据库。生产源码、默认单元测试配置与依赖锁均不修改。

## 运行

前提：Python 3、项目锁定的 Node 24.21.0、已经安装的 `web-vben` 依赖，以及与当前生产源码匹配的 `bin/imgnest-vben`。二进制需要可用的 libvips 8.18.6 运行环境（参见 `docs/development.md`）。runner 不构建、不安装依赖、不连接外部服务。

从仓库根目录执行：

```sh
python3 web-vben/e2e/run_real_api.py
```

可显式传入已有二进制和 Node；不会因此复用该二进制的默认数据库或配置：

```sh
python3 web-vben/e2e/run_real_api.py \
  --binary ./bin/imgnest-vben \
  --node /path/to/node
```

runner 在同一个命令环境中创建临时配置、SQLite 数据库、对象根目录和缩略图缓存，使用 CLI 完成生产迁移、随机管理员和本地存储初始化，再监听 `127.0.0.1:0`。普通用户与所有业务对象均通过真实前端 API 创建。退出时停止 Go 子进程、验证优雅退出并删除临时目录；持久化证据只保存已脱敏日志和测试结果。

结果默认写入 Git 忽略的 `web-vben/e2e/results/`：

- `real-api-report.json`：二进制 SHA-256、Node 版本、各项结果、真实 fetch 路径/状态、SQLite 和对象文件核对、进程/临时状态清理
- `real-api-vitest.log`：Vitest 实际输出
- `real-api-server.log`：CLI 和真实 Go 服务日志

可用 `--results /path/to/results` 调整结果目录。失败返回非零退出码；HTTP 测试通过但数据库、存储、凭证日志或清理检查失败时，整体仍为失败。

## 独立静态检查

这组测试不在默认 `src/**/*.test.ts` 范围中，普通 `pnpm test` 不会启动它。新增 TypeScript 使用独立配置检查，不能用现有前端两份配置的通过结果替代：

```sh
cd web-vben
pnpm exec vue-tsc --noEmit -p e2e/real-api.tsconfig.json
pnpm exec eslint e2e/real-api.config.ts e2e/real-api.setup.ts e2e/real-api.test.ts
pnpm exec prettier --check e2e/real-api.config.ts e2e/real-api.setup.ts e2e/real-api.test.ts e2e/real-api.tsconfig.json
```

## 实际覆盖

13 个顺序阶段共用一个一次性数据库，第一项失败即停止依赖它的后续阶段：

1. 验证码关闭配置、真实管理员/普通用户登录与注册、安全用户视图、localStorage Bearer 读取
2. 真实 400/401/404/429 和非 JSON 错误映射；20002 不触发凭证失效回调，20001 会触发；第四次登录验证原生限流，未修改限流
3. 普通用户对用户、组、存储、规则、设置、全站图片、验证码管理读取及设置写入的拒绝，原会话仍有效
4. API Token 只在创建时返回明文，列表无明文/哈希；真实 Token 可鉴权，吊销立即失效；他人 Token 不可吊销
5. 相册创建/修改、编码关键词分页、`cover_image_id=0`、非法名称和跨用户修改拒绝
6. 真实 `uploadImages` XHR：错误文件后继续两个 PNG 上传；真实 libvips、本地对象、缩略图、EXIF API、配额和概览总数
7. 相册移动的混合 207 逐项结果、计数、设置/清空封面、`album_id=0`、删册留图
8. 私有图隐藏于匿名画廊，公开图出现在画廊；安全字段；私有原图直链按产品语义仍可访问；公开缩略图匿名成功、私有缩略图匿名拒绝
9. 删除后原 URL 404、回收站时间/计数、配额与相册计数；混合 207 恢复及原图逐字节一致
10. 仅清除随机测试图片，批量删除/purge、列表和配额归零、已清理预览返回 404
11. 退出吊销当前 web Token、正确改密吊销该普通用户全部剩余 Token，管理员会话继续有效

新增检查插入对应阶段：

- 真实 searchImages/suggestAlbums 模块验证 qv=1、别名规范化、过滤结果/总数、固定相册范围、相册建议与结构化语法/范围错误
- 真实管理员 createUser/patchUser 模块验证创建、身份与昵称编辑、唯一性回滚、自我停用保护和安全响应；仅操作第三个临时测试账号

测试结束后另以 Python 只读查询 SQLite，确认三名生成账户、迁移自带的禁用 `id=0` guest 锚点、唯一剩余管理员 Token、不含 Token 明文、零图片/EXIF/相册/配额、无外键错误；遍历临时对象目录确认无残留文件。它不通过直接写库伪造任何业务状态。

## 环境与验收边界

Vitest 使用 jsdom 的真实 `localStorage`、`FormData`、`File` 和 XMLHttpRequest 网络实现。Node 原生 fetch 不支持浏览器相对 URL，因此唯一的 fetch 适配器把相对路径解析成该临时服务地址并转交原始 fetch；只允许这个 loopback origin，不改请求内容、不制造响应、不代理到外部。`real-api-report.json` 的 fetch 请求统计不包含 XHR 上传；3 次上传由真实 XHR 和 Go 服务处理。

这不是浏览器 E2E：没有启动 Chromium，没有浏览器截图、DOM 布局、交互、可访问性、CORS 或真实浏览器上传进度验收。Token 页面的离页销毁、路由与会话组件仍应由现有组件套件和可用浏览器分别验证。本轮不再尝试绕过 CUA `ERR_BLOCKED_BY_CLIENT` 或 Chromium socket `EPERM`。

本组仅覆盖 SQLite/local storage 和验证码关闭路径；不代表 PostgreSQL 驱动/并发、S3/MinIO、真实 Turnstile 供应商、Docker 或多架构验收通过。

2026-10-05 本轮实跑：13/13 阶段通过，123 次原生 fetch 网络请求与 3 次真实 XHR 上传；SQLite/对象核对、无凭证日志、优雅退出和临时目录清理均通过。运行时二进制 SHA-256 为 `5f663f9741113d7e45fc5579709914aa00f9facffdf48ca0e61f755c31b1d268`。独立 TypeScript、ESLint、Prettier 检查通过；实际输出以本地结果文件为准。
