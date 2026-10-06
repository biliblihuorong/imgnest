# M5 双前端构建记录

## 基线与边界

- 实际上游 M5：`d19323d2714f9d050ffcde8221ef2738e3e98e53`
- 隔离快照：`b67f5c9ad5d2ef2be3f07176813cf004f997cc5e`
- 原 web tree：`69c6205f64ee54fc5a581eb1a7df32cd9066d25a`
- 原 `web/` 所有非生成文件维持该基线；102 项清单来自快照的 Git blob，见 `legacy-m5-source.sha256`
- Go 业务路由不变；`serve.go` 仅从直接调用 `web.DistFS` 改为 `frontendDistFS`，删除原直接 import

## 实现

- `frontend_legacy.go` 的 `!vben` 和 `frontend_vben.go` 的 `vben` 互斥；只选择一个 embed 包
- 新 `web-vben/embed.go` 嵌入自己的 dist，旧 `web/embed.go` 未改
- `fe-build` / `release` 默认 legacy；两种 release 输出分别为 `bin/imgnest-legacy` 与 `bin/imgnest-vben`
- 普通构建只 frozen install + build，不执行 `gen:api`；新类型生成独立为 `fe-gen-api-vben`
- 构建前后执行冻结源文件检查，不读取或安装另一套前端依赖
- 新 `.gitignore` 包括 Vben 根与 vendor workspace 的 node_modules、dist 和 coverage

## 可重复检查

```sh
node scripts/check-legacy-source.mjs
node --test scripts/check-legacy-source.test.mjs
python3 scripts/test-frontend-builds.py
sh scripts/check-frontend-selection.sh
make release-legacy
make release-vben
```

前两项真正读取/改变隔离测试夹具验证完整性检查；Python 检查只测试 Make 展开后的命令路由，不声称实际安装或编译。Go 脚本真正加载服务依赖图并编译 selector 及其 embed 包，比较实际嵌入文件与被选 dist 的完整字节集合；完整 CLI 的 libvips 链接、HTTP 联调是另外的关口。

## 本轮环境和证据（2026-10-05）

官方 Go 1.27.1 Linux amd64 已从 `https://go.dev/dl/go1.27.1.linux-amd64.tar.gz` 获取并安装在独立临时目录；SHA-256 为 `63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445`，对照 `https://go.dev/dl/?mode=json` 验证通过。系统 `/usr/bin/go` 是同名棋类程序，未使用或替换。测试设置独立 GOCACHE/GOMODCACHE 和 `GOTOOLCHAIN=local`；没有改系统权限。

TDD 记录：新增 Make 路由检查最初五项失败（目标不存在），新增 selector 测试最初因缺失函数失败；实现后五项 Make 检查、四项源文件完整性夹具检查、两个 selector 的真实 Go 测试通过。冻结源清单检查通过（102 项）。完整 release 和真实服务验证在有对应 libvips/运行条件后单独记录，不能以这些定向检查代替。

## web-vben 内的两套外壳（2026-10-06）

`web-vben/` 现在包含两套外壳，同一份构建产物里都有，运行时切换，不需要重新构建：

- **Marvis（默认）**：漂浮侧栏、`Ctrl/⌘ K` 全局检索、设置弹窗、宽屏下「我的图片」右侧详情面板。
- **经典**：原 Vben 布局（`ClassicLayout.vue`，由原 `AppLayout.vue` 原样搬入）。

`AppLayout.vue` 是分发器，两套布局各自成独立 chunk，按需加载。外壳选择存在 `localStorage["imgnest-shell"]`（`marvis` / `classic`），未设置或值非法时用 `marvis`。深浅色、强调色、圆角、侧栏折叠沿用 Vben 偏好（`imgnest-vben-v1-preferences*`）；强调色与圆角在用户没自选时跟随外壳默认值，自选后两套外壳共用。

切换入口：Marvis 侧栏底部齿轮 → 设置 → 外观；经典布局的用户下拉菜单 →「切换到新版布局」。

这不改变发布默认：默认二进制仍嵌 `web/`，`make release-vben` 才包含上述两套外壳。设计与计划见 `docs/superpowers/specs/2026-10-06-marvis-shell-design.md`、`docs/superpowers/plans/2026-10-06-marvis-shell.md`，验收记录见 `marvis-shell-validation.md`。
