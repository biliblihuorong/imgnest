# 浏览器回归与真实 API 联调

## CI 使用的生产构建浏览器回归

`check-access.mjs` 对真实生产前端运行 Chromium，API 使用脚本内的隔离夹具。它只接受本机 origin，测试账号、设置和图片只在内存中存在，不连接生产账户或存储。

先安装项目锁定依赖并构建 `web-vben`，再从仓库根执行：

```bash
python3 .claude/skills/webapp-testing/scripts/with_server.py \
  --server 'node web-vben/e2e/serve-build.mjs' --port 4173 \
  -- node web-vben/e2e/check-access.mjs
```

浏览器工具沿用外部 Playwright 1.62.1；用 `IMGNEST_E2E_PLAYWRIGHT_MODULE` 指向其 `index.mjs`，或让 Node 能解析 `playwright`。已有 Chromium 可加 `--chromium /path/to/chromium`。默认结果写入 `/tmp/imgnest-access-browser`，可用 `--output` 更改。

覆盖：余额购买的桌面/手机确认、丢失响应后重试不重复扣款、刷新恢复原订单、管理员先核对后入账、普通版隐藏账务入口；以及画廊与游客上传的四种组合、未配置游客组、首页失败重试、长站点名移动登录页、管理员按未保存范围预览和显式保存、备份图片排除提示、桌面与手机相册详情的随机链接启用入口、图片页回收站标签。未知夹具 API 和浏览器异常会使脚本失败，每个场景保存截图及 JSON 结果。界面与交互通过仍不代表真实后端已通过。

CI 的 Browser regressions job 还构建文档站并运行 [`website/e2e/check-layout.mjs`](../../website/e2e/check-layout.mjs)，检查中英文任务导航、随机链接创建与相册前置步骤的可达性，以及不同视口下的目录布局。两个脚本的截图与结果都上传为 `browser-regressions`，需人工查看实际页面，不能只看断言。

## 其他页面的开发夹具

原有 `check_ui.py` 和 `fixtures/server.mjs` 保留用于更广的页面巡查，不作为上述 CI 的替代。它需要外部 Python Playwright：

```bash
python3 .claude/skills/webapp-testing/scripts/with_server.py \
  --server 'cd web-vben && pnpm dev --host 127.0.0.1 --port 5173 --strictPort' --port 5173 \
  --server 'node web-vben/e2e/fixtures/server.mjs' --port 8088 \
  -- python3 web-vben/e2e/check_ui.py
```

该脚本使用另一套一次性本机接口夹具，结果写入 `web-vben/e2e/results/`。按具体变更补充语言持久化、弹窗、上传、相册移动、回收站、Token 和验证码等人工交互验收，不把截图巡查当作全功能验收。

## 真实前端 API 与 Go 联调

[真实 API 运行说明](REAL_API.md) 使用实际 Go 二进制、SQLite 迁移、本机存储与 libvips，执行真实 TypeScript API 模块，不启动浏览器。它与浏览器夹具互补；权限和数据过滤还需通过 Go HTTP/service/repo 测试验证。
