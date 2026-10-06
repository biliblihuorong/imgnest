# 隔离前端 E2E 与截图

这是使用一次性本地接口夹具的自动化检查，不连接生产站点，不替代真实 Go API 联调。数据、密码和 Token 均为测试占位值，只存在进程内存，停止后丢弃。

本次编写环境未能启动浏览器：Chromium 的私有进程 socket 返回 EPERM；受支持的沙箱外重试也在运行时挂载阶段失败。因此 `check_ui.py` 已准备但未通过执行，不附虚构的截图。单元/组件测试和真实 Go 检查的结果见交付验证报告。

在允许本地 Chromium 的开发环境，安装 Python Playwright 和其官方 Chromium 后，从仓库根运行（先安装 web-vben 的锁定依赖）：

```sh
python .claude/skills/webapp-testing/scripts/with_server.py \
  --server 'cd web-vben && pnpm dev --host 127.0.0.1 --port 5173 --strictPort' --port 5173 \
  --server 'node web-vben/e2e/fixtures/server.mjs' --port 8088 \
  -- python web-vben/e2e/check_ui.py
```

已有受支持 Chromium 时可添加 `--chromium /path/to/chromium`。结果写入 `web-vben/e2e/results/`。只允许默认本地夹具 origin，脚本不会把假登录信息发送到真实站点。

脚本覆盖登录入口、普通/管理员默认首页、普通用户管理路由拒绝、主要页面、匿名画廊与移动视口截图。进一步人工验收需按任务书执行语言持久化、打开弹窗时切换语言、重复操作、取消/后退、实际上传与相册移动/回收、一次性 Token、服务错误和验证码全生命周期；不应把截图 smoke 当成所有交互已通过。

## 真实前端 API 与 Go 联调（不启动浏览器）

独立的 [真实 API 运行说明](REAL_API.md) 使用现有 Go 二进制、生产 SQLite 迁移、本地存储与 libvips，直接执行 `src/api` 的真实 TypeScript 模块。它和上面的 mock 浏览器夹具是两套独立测试；真实 HTTP 联调通过不代表浏览器渲染或交互验收通过。
