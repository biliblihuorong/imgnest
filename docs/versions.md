# 版本矩阵（锁定于 2026-10-04）

这些版本锁定于 2026-10-04，已核对正式发布 tag 与关键 `peerDependencies` / `go` 指令兼容性；它们不必是各项目当前最新版本。
升级任何一项都要同步改这张表，并在 PR 里说明原因。

> 注意：核对时 npm 与 Go 官方代理在我这边不可访问，所以这里只验证了「tag 存在 + 依赖声明兼容」。第一次 `pnpm install` / `go mod tidy` 时请确认能装上，然后提交 `pnpm-lock.yaml` 与 `go.sum`，以锁文件为准。

执行补充：官方注册表元数据可访问；M1/M2 实际选择记录在 `go.mod` / `go.sum`。AWS SDK、imagemeta、vipsgen 已安装并实际链接固定 libvips，前端仍待 M3 安装；没有升级已锁定直接依赖。M2 显式使用 SDK 自带错误类型，因此 smithy-go v1.28.1 从 SDK 的间接依赖提升为直接依赖；x/sync v0.23.0 由 tidy 保留为间接依赖。

M3 执行补充：前端依赖按上表精确锁定并提交 `web/pnpm-lock.yaml`。Node 24.21.0 与 pnpm 12.9.1（`npm i -g` 固定）已装入 dev 镜像，pnpm store 固定到 compose 卷（`web/pnpm-workspace.yaml` storeDir）；宿主 Node 22 仅作手工便利。实测 pnpm 12 忽略 `npm_config_store_dir` 环境变量，须用 pnpm-workspace.yaml 配置。`openapi-typescript@7.13.0` 声明 peer typescript ^5.x，与锁定的 TS 6.0.3 组合实测 `gen:api` 正常，属可接受取舍；为避免引入锁外依赖（jiti、@types/node），ESLint 配置用 `.mjs`、vite alias 用 `import.meta.url` 解析（Windows 宿主直跑 dev 有已知限制，验收在容器内）。@vueuse/core 已安装，M3 暂无使用点，主题跟随系统暂用 Naive UI 内置 `useOsTheme`。

M4 执行补充：无新增前后端依赖。`/api/v1` 与 `/api/admin` 契约已全部写入 `docs/openapi.yaml`；管理端 api 模块（web/src/api/admin.ts）暂为手写类型，后续可切 gen:api 派生。

## 工具链

| 工具 | 版本 | 说明 |
| --- | --- | --- |
| Go | 1.27.1 | `go.mod` 写 `go 1.27.0` + `toolchain go1.27.1`；imagemeta 要求 Go ≥ 1.27 |
| Node.js | 24.21.0（LTS） | vitest 5 要求 `^22.12 \|\| ^24 \|\| >=26`；`.nvmrc` 写 `24.21.0` |
| pnpm | 12.9.1 | `package.json` 写 `"packageManager": "pnpm@12.9.1"` |
| libvips | 8.18.6 | 与 `vipsgen/vips` 包对应，必须一致 |
| Docker 基础镜像 | `ghcr.io/cshum/imagor-base:vips8.18.6-r14`（运行）/ `-dev`（构建） | M2 dev 镜像基于 -dev 重编同版本 vips，启用 BMP 所需 Magick；见下文 |
| golangci-lint | v2.14.0 | 配置文件用 v2 格式 |

## Go 依赖

| 模块 | 版本 | 备注 |
| --- | --- | --- |
| github.com/gin-gonic/gin | v1.12.0 | |
| gorm.io/gorm | v1.31.2 | |
| gorm.io/driver/postgres | v1.6.3 | M1 实际锁文件选择 jackc/pgx/v5 v5.10.0（校正原备注） |
| gorm.io/driver/sqlite | v1.6.0 | cgo；M1 实际锁文件选择 mattn/go-sqlite3 v1.14.22（校正原备注） |
| github.com/cshum/vipsgen | v1.3.11 | 只导入 `vipsgen/vips`（libvips 8.18.x） |
| github.com/evanoberholster/imagemeta | v1.1.0 | EXIF 读取 |
| github.com/aws/aws-sdk-go-v2 | v1.47.1 | |
| github.com/aws/aws-sdk-go-v2/config | v1.33.6 | |
| github.com/aws/aws-sdk-go-v2/credentials | v1.20.6 | |
| github.com/aws/aws-sdk-go-v2/service/s3 | v1.114.0 | 校验和设为 `WhenRequired` |
| github.com/aws/smithy-go | v1.28.1 | 固定 SDK 所选错误类型依赖，M2 显式导入 |
| github.com/knadh/koanf/v2 | v2.3.7 | 配置；子模块见下 |
| github.com/knadh/koanf/providers/file | v1.2.1 | |
| github.com/knadh/koanf/providers/env/v2 | v2.0.1 | |
| github.com/knadh/koanf/providers/confmap | v1.0.1 | |
| github.com/knadh/koanf/parsers/yaml | v1.1.1 | |
| github.com/spf13/cobra | v1.10.2 | 命令行 |
| golang.org/x/crypto | v0.57.0 | bcrypt |
| golang.org/x/sync | v0.23.0 | semaphore / errgroup |
| github.com/stretchr/testify | v1.12.1 | 测试 |

可直接粘贴的 `go.mod` 片段：

```text
go 1.27.0

toolchain go1.27.1

require (
	github.com/aws/aws-sdk-go-v2 v1.47.1
	github.com/aws/aws-sdk-go-v2/config v1.33.6
	github.com/aws/aws-sdk-go-v2/credentials v1.20.6
	github.com/aws/aws-sdk-go-v2/service/s3 v1.114.0
	github.com/cshum/vipsgen v1.3.11
	github.com/evanoberholster/imagemeta v1.1.0
	github.com/gin-gonic/gin v1.12.0
	github.com/knadh/koanf/parsers/yaml v1.1.1
	github.com/knadh/koanf/providers/confmap v1.0.1
	github.com/knadh/koanf/providers/env/v2 v2.0.1
	github.com/knadh/koanf/providers/file v1.2.1
	github.com/knadh/koanf/v2 v2.3.7
	github.com/spf13/cobra v1.10.2
	github.com/stretchr/testify v1.12.1
	golang.org/x/crypto v0.57.0
	golang.org/x/sync v0.23.0
	gorm.io/driver/postgres v1.6.3
	gorm.io/driver/sqlite v1.6.0
	gorm.io/gorm v1.31.2
)
```

## 前端依赖

| 包 | 版本 | 备注 |
| --- | --- | --- |
| vue | 3.5.43 | |
| vue-router | 5.3.1 | peer：vue ≥3.5.34、pinia ^3.0.4 \|\| ^4.0.2、vite ^7.3 \|\| ^8 |
| pinia | 4.0.3 | peer：vue ^3.5.11 |
| naive-ui | 2.45.3 | node ≥ 20 |
| @vueuse/core | 15.0.0 | |
| vite | 8.3.2 | Rolldown 版；node ^20.19 \|\| ≥22.12 |
| @vitejs/plugin-vue | 6.0.9 | peer：vite ^5–^8 |
| typescript | **6.0.3** | **不要用 7.x**：typescript-eslint 8.71.0 要求 `typescript <6.1.0` |
| vue-tsc | 3.3.12 | |
| @vue/tsconfig | 0.9.1 | |
| eslint | 10.12.0 | flat config（`eslint.config.ts` / `.mjs`） |
| eslint-plugin-vue | 10.11.1 | |
| @vue/eslint-config-typescript | 14.9.0 | 内含 typescript-eslint |
| typescript-eslint | 8.71.0 | |
| @vue/eslint-config-prettier | 10.2.0 | |
| prettier | 3.9.9 | |
| vitest | 5.0.3 | |
| @vue/test-utils | 2.5.1 | |
| jsdom | 30.1.1 | vitest 环境 |
| @playwright/test | 1.63.0 | E2E |
| openapi-typescript | 7.13.0 | 由 `docs/openapi.yaml` 生成前端类型 |

可直接粘贴的 `package.json` 片段（精确版本，不用 `^`）：

```json
{
  "packageManager": "pnpm@12.9.1",
  "engines": { "node": ">=24.21.0 <25" },
  "dependencies": {
    "@vueuse/core": "15.0.0",
    "naive-ui": "2.45.3",
    "pinia": "4.0.3",
    "vue": "3.5.43",
    "vue-router": "5.3.1"
  },
  "devDependencies": {
    "@playwright/test": "1.63.0",
    "@vitejs/plugin-vue": "6.0.9",
    "@vue/eslint-config-prettier": "10.2.0",
    "@vue/eslint-config-typescript": "14.9.0",
    "@vue/test-utils": "2.5.1",
    "@vue/tsconfig": "0.9.1",
    "eslint": "10.12.0",
    "eslint-plugin-vue": "10.11.1",
    "jsdom": "30.1.1",
    "openapi-typescript": "7.13.0",
    "prettier": "3.9.9",
    "typescript": "6.0.3",
    "typescript-eslint": "8.71.0",
    "vite": "8.3.2",
    "vitest": "5.0.3",
    "vue-tsc": "3.3.12"
  }
}
```

建议在 `.npmrc` 里加 `save-exact=true`，之后 `pnpm add` 也写精确版本。

## 数据库与周边

| 组件 | 建议 | 说明 |
| --- | --- | --- |
| PostgreSQL | 18.6（18 系列最新） | 主力；EXIF `raw` 用 JSONB。19 目前还是 beta，先不用 |
| SQLite | 跟随 mattn/go-sqlite3 内置版本 | 开 WAL + `busy_timeout`，单写连接 |
| MinIO | RELEASE.2025-10-15T17-29-55Z | 官方正式源码 tag；提交9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a，隔离测试专用 |

M2 的实际 native 验证为 Go1.27.1 / vips8.18.6。原 imagor-base 禁用了 Magick，无法加载有效 BMP；开发 Dockerfile 使用官方 vips8.18.6 tarball（SHA256 `3c41e1d5458081bfa4a5bc54e116c46259c75c6760a18027764555632b9dda3e`）重编，保持版本并启用 Magick。构建工具来自镜像 Ubuntu noble：meson1.3.2-1ubuntu1、ninja-build1.11.1-2；它们不属于产品 Go 依赖。实际使用 jemalloc，关闭 libvips 操作缓存。M2 验收覆盖当前 Docker Linux amd64；arm64 发布镜像属于后续发布关口。

## Vben Naive UI 迁移（2026-10-05）

授权的前端框架迁移引入真实 Vben 5.8.0 源码工作区：官方仓库 `vbenjs/vue-vben-admin`，提交 `50f4ede309d4450c7dd417399cb8d5c02346d2d2`。源码、MIT 全文与逐文件上游 SHA-256 记录位于 `web-vben/vendor/vben/`。仅保留 24 个所需运行包及 Tailwind 构建插件，不包含上游应用、演示、后端或其他 UI 库应用。

保留现有精确版本：Vue 3.5.43、Vue Router 5.3.1、Pinia 4.0.3、Naive UI 2.45.3、VueUse 15.0.0、Vite 8.3.2、TypeScript 6.0.3 与 Vitest 5.0.3。未降级上述依赖；`@vue/shared` 对齐 Vue，`@vueuse/integrations` 对齐既有 VueUse 15。新增 registry 依赖使用上游锁文件的精确版本（见下表），全部解析及间接版本以 `web-vben/pnpm-lock.yaml` 为准。

| 新增依赖 | 精确版本 |
| --- | --- |
| `@ctrl/tinycolor` | 4.2.0 |
| `@iconify/json` | 2.2.520 |
| `@iconify/tailwind4` | 1.2.3 |
| `@iconify/vue` | 5.0.1 |
| `@intlify/core-base` | 11.4.10 |
| `@lucide/vue` | 1.34.0 |
| `@tailwindcss/typography` | 0.5.20 |
| `@tailwindcss/vite` | 4.3.3 |
| `@tanstack/store` | 0.11.1 |
| `@tanstack/vue-form` | 1.33.5 |
| `@tanstack/vue-store` | 0.11.1 |
| `@types/json-bigint` | 1.0.4 |
| `@types/lodash.clonedeep` | 4.5.9 |
| `@types/nprogress` | 0.2.3 |
| `@types/qrcode` | 1.5.6 |
| `@types/qs` | 6.15.1 |
| `@types/sortablejs` | 1.15.9 |
| `@vue/shared` | 3.5.43 |
| `@vueuse/integrations` | 15.0.0 |
| `class-variance-authority` | 0.7.1 |
| `clsx` | 2.1.1 |
| `dayjs` | 1.11.23 |
| `defu` | 6.1.7 |
| `es-toolkit` | 1.51.0 |
| `json-bigint` | 1.0.0 |
| `lodash.clonedeep` | 4.5.0 |
| `nprogress` | 0.2.0 |
| `pinia-plugin-persistedstate` | 4.7.1 |
| `qrcode` | 1.5.4 |
| `qs` | 6.15.3 |
| `reka-ui` | 2.10.4 |
| `sass` | 1.103.1 |
| `secure-ls` | 2.0.0 |
| `sortablejs` | 1.15.7 |
| `tailwind-merge` | 3.6.0 |
| `tailwindcss` | 4.3.3 |
| `theme-colors` | 0.1.0 |
| `tippy.js` | 6.3.7 |
| `tw-animate-css` | 1.4.0 |
| `vue-i18n` | 11.4.10 |
| `vue-json-pretty` | 2.6.0 |
| `vue-tippy` | 6.7.1 |
| `watermark-js-plus` | 1.6.6 |
| `zod` | 4.4.3 |
| `zod-defaults` | 0.2.3 |

Vben 的 layout、menu、tabs 与通用控件来自 vendored 源码；业务表格、弹窗、表单继续使用 Naive UI。Vben 样式采用其原生 Tailwind 4 + Reka/shadcn 内核，保留上游 design tokens 与明暗主题。新版输出独立位于 `web-vben/dist`，legacy 的 `web/dist` 与 package/lock/config保持实际M5基线不变。独立工作区不包含旧前端；默认Go构建仍使用legacy，`-tags=vben`选择新版。详见双前端构建说明。

## CI 缓存执行方式（2026-10-06）

工具链和依赖版本不变。`deploy/Dockerfile.dev` 的默认 `dev` target 保留原开发环境；可选 `lint` target 使用 Go 1.27.1 编译同一个 golangci-lint v2.14.0，只复制二进制，不把安装器的模块/编译缓存带进镜像。

GitHub Actions 分别缓存 dev、lint、固定源码版本的 MinIO 镜像。`deploy/compose.ci.yaml` 仅在 CI 使用：显式复用已加载镜像，将容器实际使用的 Go 模块/编译缓存及 lint 缓存绑定到 runner 临时目录，再由 `actions/cache@v5` 跨运行保存。backend/lint 使用独立缓存键，包含 OS、架构、工具链/原生依赖声明、Go 依赖、lint 配置和提交；兼容前缀用于恢复前一提交的编译结果。缓存不包含数据库、对象存储数据、工作区或认证信息。PR 缓存遵循 GitHub 的 merge-ref 隔离规则，合并后的 main 首次运行需建立 main 的缓存。

完整质量门禁保持不变：`go vet`、`go test -race -shuffle=on -count=1 -timeout 40m ./...`、SQLite/PostgreSQL/MinIO 集成、两种前端的类型检查/测试/构建、前端选择检查和两种 Go 二进制构建。`-count=1` 保证每次重新执行测试，缓存只减少依赖下载和编译。生产 bcrypt cost=12 不变。较便宜的前端选择检查前移，避免在长测试结束后才发现选择错误。

`python3 scripts/test-ci-config.py` 检查缓存接线及原有命令；CI 使用 `--compose` 额外检查 Docker Compose 实际合并后挂载与测试服务配置。比较性能时应分开记录首次构建和相同提交的热缓存重跑，不能把缓存命中推断为测试已执行。

CI 实测补充：相同源码两轮构建中，`golang:1.27.1-bookworm` tag 解析到了不同摘要，导致 MinIO 与 lint 安装层重新执行。因此两个 Go 构建阶段进一步固定 `sha256:8d48e12ec56735e9358640898b9d9b9fcca110612ed8a5567438c0a1baa24e66`，Go 版本仍为 1.27.1。MinIO 与 lint 在安装命令的同一个 RUN 中移除安装器的模块/编译缓存，避免将无用缓存写入镜像层后再导出到 Actions cache。后续更换摘要需显式更新并重新跑完整 CI。
