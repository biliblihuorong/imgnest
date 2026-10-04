# 版本矩阵（锁定于 2026-10-04）

所有版本号都是 2026-10-04 从各项目 GitHub 正式发布 tag 核对的最新稳定版，并检查过彼此的 `peerDependencies` / `go` 指令兼容性。
升级任何一项都要同步改这张表，并在 PR 里说明原因。

> 注意：核对时 npm 与 Go 官方代理在我这边不可访问，所以这里只验证了「tag 存在 + 依赖声明兼容」。第一次 `pnpm install` / `go mod tidy` 时请确认能装上，然后提交 `pnpm-lock.yaml` 与 `go.sum`，以锁文件为准。

## 工具链

| 工具 | 版本 | 说明 |
| --- | --- | --- |
| Go | 1.27.1 | `go.mod` 写 `go 1.27.0` + `toolchain go1.27.1`；imagemeta 要求 Go ≥ 1.27 |
| Node.js | 24.21.0（LTS） | vitest 5 要求 `^22.12 \|\| ^24 \|\| >=26`；`.nvmrc` 写 `24.21.0` |
| pnpm | 12.9.1 | `package.json` 写 `"packageManager": "pnpm@12.9.1"` |
| libvips | 8.18.6 | 与 `vipsgen/vips` 包对应，必须一致 |
| Docker 基础镜像 | `ghcr.io/cshum/imagor-base:vips8.18.6-r14`（运行）/ `-dev`（构建） | 方案 A，见 spec 3.1 |
| golangci-lint | v2.14.0 | 配置文件用 v2 格式 |

## Go 依赖

| 模块 | 版本 | 备注 |
| --- | --- | --- |
| github.com/gin-gonic/gin | v1.12.0 | |
| gorm.io/gorm | v1.31.2 | |
| gorm.io/driver/postgres | v1.6.3 | 间接引入 jackc/pgx/v5 v5.11.0 |
| gorm.io/driver/sqlite | v1.6.0 | cgo，底层 mattn/go-sqlite3 v1.14.52 |
| github.com/cshum/vipsgen | v1.3.11 | 只导入 `vipsgen/vips`（libvips 8.18.x） |
| github.com/evanoberholster/imagemeta | v1.1.0 | EXIF 读取 |
| github.com/aws/aws-sdk-go-v2 | v1.47.1 | |
| github.com/aws/aws-sdk-go-v2/config | v1.33.6 | |
| github.com/aws/aws-sdk-go-v2/credentials | v1.20.6 | |
| github.com/aws/aws-sdk-go-v2/service/s3 | v1.114.0 | 校验和设为 `WhenRequired` |
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
| MinIO | 最新稳定版 | 仅用于 S3 驱动集成测试 |
