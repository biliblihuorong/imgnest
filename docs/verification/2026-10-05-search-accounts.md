# 相册、统一搜索与账户管理交付验证

基线：`f25a448a1078663095a7450994b4de4348f704f0`，`feat/vben-ui-migration`。
目标独立分支：`feat/unified-search-accounts-20261005`。
日期：2026-10-05 UTC。

## 交付内容

- 无相册空态打开已有新建相册表单；已有空相册、空页和查询无结果不混淆；失效末页只回退一次。
- 我的图片与相册详情共用 TXT 1.1 统一搜索：严格语法和别名、条件标签、相册补全、? 帮助、输入法和键盘操作、草稿/已应用状态、URL/浏览器历史、取消/超时/迟到响应防护。
- Go 与 TypeScript 共享解析样例；新 qv=1 协议在数据库完整授权范围内先过滤/COUNT，再稳定排序与分页；旧请求兼容。日期筛上传 created_at；MB 为十进制；格式来自可信实际 MIME。
- 相册详情的路径 ID 是不可被查询清除的服务端边界，ID 在新搜索链路保持十进制字符串精度。缺失和不可访问相册不区分返回；补全与错误候选受同一范围限制。
- 管理员创建用户并编辑用户名、邮箱、昵称、角色、状态和用户组；初始密码复用原校验/bcrypt；已有密码、容量、ID、头像配置和时间字段不能通过用户 PATCH 改写。
- 自我停用、并发最后启用管理员、唯一性与事务回滚保护；身份/权限变化撤销令牌，持久化 auth_version 防止字段改回原值后旧登录证明恢复有效。
- 自助用户名/邮箱仍只读；昵称可重复、空值回退用户名。自助资料请求必须明确提供非 null 字符串，空字符串仍可主动清除昵称。
- 上传、复制菜单、灯箱、头像、存储修复及旧 web 保留；所有依赖版本/锁文件未改。

## 最终自动化结果

| 检查 | 结果 |
| --- | --- |
| 新版前端完整 Vitest | 61 文件、708 测试通过 |
| 新版 vue-tsc（两份配置） | 通过 |
| 新版 ESLint | 0 错误；4 个原有测试文件多组件警告 |
| 新版生产构建 | 通过；原有 Vben 大 chunk 提示仍存在 |
| 旧前端 Vitest | 37 文件、304 测试通过 |
| 旧前端 typecheck/lint/build | 通过 |
| 冻结源码检查 | 102 个旧前端受保护文件完全一致 |
| Go 全量 race，SQLite + PostgreSQL | 16 个测试包通过，最终失败 0；跳过项见下方 |
| Go 模块校验 | 全部模块 verified |
| golangci-lint 2.14.0 全仓库 | 0 issues |
| gofmt / git diff --check | 通过 |
| Go legacy / vben 二进制构建 | 均通过 |
| 双前端选择与源码保护脚本 | 4 个 Node 测试、5 个 Python 测试及选择器检查通过 |
| integration 标签相册/搜索 HTTP 子集 | 通过 |
| 真实前端 API → Go HTTP → SQLite/local | 13/13 阶段通过，123 次 fetch 与 3 次真实 XHR 上传 |
| 独立解析器差异检查 | 21,192 个固定/种子样例，AST、接受/拒绝、规范串、错误码与 UTF-16 span 差异为 0 |
| Go 解析器 fuzz | 99,429 次执行通过；解析器覆盖率 96.7% |

最终完整 Go race 命令为：

```sh
go test -mod=readonly -race -count=1 -timeout=30m -json ./...
golangci-lint run ./...
go mod verify
```

使用项目锁定的 Go 1.27.1、Node 24.21.0、pnpm 12.9.1、libvips 8.18.6、PostgreSQL 18.6，数据库为隔离临时测试数据。服务包 race 耗时 676.598 秒；密码测试保留真实 bcrypt 成本，没有为通过检查降低生产参数。最后一轮执行在产品源码冻结后启动。

前端与真实 API 检查：

```sh
pnpm --dir web-vben test
pnpm --dir web-vben typecheck
pnpm --dir web-vben lint
pnpm --dir web-vben build
pnpm --dir web test
pnpm --dir web typecheck
pnpm --dir web lint
pnpm --dir web build
python3 web-vben/e2e/run_real_api.py --binary ./bin/imgnest-vben
```

真实 API 检查包含新增 searchImages/suggestAlbums/createUser/patchUser 包装器，使用真实 Go 服务和生产 SQLite 迁移。结束时核对：三名测试账号与禁用的 guest 锚点、仅剩管理员 Token、图片/EXIF/相册/容量及对象文件归零、无外键错误、无明文 Token、服务正常退出、临时数据清理。没有连接真实账号或生产存储。

通过真实 API 检查的 Vben 二进制 SHA-256：
`5f663f9741113d7e45fc5579709914aa00f9facffdf48ca0e61f755c31b1d268`。

## 搜索性能

隔离 Linux amd64、9 个逻辑 CPU、Go 1.27.1，1 万/10 万条合成记录，两个用户 90/10 分布、多个相册、同名与未知格式数据；预热后 21 次测量，每页 20，包含准确 COUNT。不是生产容量承诺。

| 10 万条场景 p95 | SQLite | PostgreSQL 18.6 |
| --- | ---: | ---: |
| 空查询 | 12.4 ms | 28.2 ms |
| 格式 + 日期 | 103.2 ms | 25.9 ms |
| 相册 | 4.6 ms | 16.0 ms |
| 稀有词 | 389.4 ms | 82.5 ms |
| 无命中词 | 371.7 ms | 69.4 ms |

这些实测场景满足建议的 500 ms p95 目标。原始 COUNT 时间和实际 EXPLAIN/扫描计划见 [性能记录](2026-10-05-search-performance.txt)，fuzz 输出见 [fuzz 记录](2026-10-05-search-fuzz.txt)。

复现性能测试：

```sh
IMGNEST_BENCH_SEARCH=1 go test ./internal/repo -run '^TestSearchPerformance$' -count=1 -v
```

## 明确未完成或未运行的验证

1. 浏览器真实渲染、截图、手机布局及浏览器端可访问性/E2E 未通过验收。Chromium 的进程 socket 在普通及受支持的升级执行中均被运行环境拒绝；云浏览器访问本地测试站点也被阻止。没有绕过这些限制。jsdom 键盘/IME/帮助/状态测试不能替代像素和真实浏览器检查。
2. 五个 MinIO/S3 集成用例被跳过。固定版本 MinIO 成功编译，但启动的 netlink 操作在受支持重试后仍被运行环境拒绝；没有改动 MinIO 或降低安全设置。这五项不能算作通过。
3. 全量命令中的 TestSearchPerformance 默认跳过，但已经按上面的显式开关在两个数据库单独执行并通过。既有锁 SQL 生成测试的 PostgreSQL 子项按设计跳过，使用 SQLite 事务承载 PostgreSQL SQL 构造器；对应其它真实 PostgreSQL 回归已运行。
4. 额外的“请求已鉴权、提交前撤销源令牌”并发场景未完成验证；不能把它视为已通过。已确认的字段 A→B→A 登录证明失效问题则已通过 auth_version 修复，并在两个数据库中验证。
5. 未执行生产迁移、合并或部署。代码交付与以上自动化结果，不等于宣布所有发布验收完成。

## 迁移和回退

- 0005：增加规范化搜索列与索引；显式 migrate 按 250 行事务分批回填，NULL 标记未完成，支持中断后继续；服务启动拒绝不完整回填。原文件名/EXIF/相册名称不改写。
- 0006：增加内部 auth_version，旧用户默认 0；有效的身份/权限修改在同一事务中递增版本并撤销 Token。昵称/无变化/失败回滚不递增。
- 升级前停止写入、备份数据库与对应存储/配置，在副本验证迁移后再安排上线。简单换回旧二进制不安全，因为它会拒绝未知迁移版本；安全回退需要恢复升级前数据库快照和匹配旧二进制。备份后新增的数据必须先明确保留/对账方案。

协议与操作说明见 [统一搜索](../unified-search.md) 和 [OpenAPI](../openapi.yaml)。
