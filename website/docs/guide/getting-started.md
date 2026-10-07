# 快速开始

这一页用 Docker 镜像和 SQLite 把 ImgNest 跑起来，并上传第一张图。

## 准备

- Docker，带 Compose 插件
- 仓库里的 compose 文件：`git clone https://github.com/biliblihuorong/imgnest`

所有命令都在仓库根目录执行。镜像是 `ghcr.io/biliblihuorong/imgnest`，支持 linux/amd64 和 linux/arm64。

## 1. 启动服务

::: code-group

```bash [Bash]
IMGNEST_TAG=edge docker compose -f deploy/compose.sqlite.yaml up -d
```

```powershell [PowerShell]
$env:IMGNEST_TAG = 'edge'
docker compose -f deploy/compose.sqlite.yaml up -d
```

:::

容器启动时会自动执行数据库迁移。数据存在名为 `imgnest-data` 的数据卷里，包括数据库、本机图片和缩略图缓存。

::: tip 为什么是 edge
`edge` 跟随 `main` 分支的最新提交。正式版本发布之后会有 `latest` 和带版本号的 tag，那时去掉 `IMGNEST_TAG` 即可。
:::

## 2. 创建管理员

密码从标准输入读取，不会出现在命令参数或日志里。密码长度为 12 到 72 个字节。

::: code-group

```bash [Bash]
read -rsp '管理员密码: ' password; echo
printf '%s' "$password" | docker compose -f deploy/compose.sqlite.yaml exec -T imgnest \
  imgnest init-admin --username admin --email admin@example.com
unset password
```

```powershell [PowerShell]
$password = Read-Host '管理员密码' -AsSecureString
$plain = [System.Net.NetworkCredential]::new('', $password).Password
$plain | docker compose -f deploy/compose.sqlite.yaml exec -T imgnest `
  imgnest init-admin --username admin --email admin@example.com
Remove-Variable plain, password
```

:::

把用户名和邮箱换成你自己的。重复执行不会覆盖已有的管理员。

## 3. 创建本机存储

```bash
docker compose -f deploy/compose.sqlite.yaml exec imgnest \
  imgnest init-local --base-url http://localhost:8080
```

这条命令会创建一个本机存储和一条默认上传规则，并绑定到默认用户组。`--base-url` 填别人访问你图床时用的地址，正式部署时换成你的域名。

## 4. 登录并上传

打开 <http://localhost:8080>，用刚才的邮箱和密码登录，进入「上传图片」，拖一张图进去。上传完成后你会拿到原图、WebP 和缩略图三个地址，默认复制的是 WebP 地址。

检查服务是否就绪：

```bash
curl http://localhost:8080/healthz
```

::: tip 注册默认关闭
新装好的站点不开放注册。需要的话，管理员可以在「站点管理 → 站点设置」里打开。
:::

## 常用操作

| 想做的事 | 做法 |
| --- | --- |
| 换端口 | 启动前设置 `IMGNEST_PORT`，默认 8080 |
| 停止服务 | `docker compose -f deploy/compose.sqlite.yaml down` |
| 改用 PostgreSQL | 用 `deploy/compose.postgres.yaml`，先设置 `POSTGRES_PASSWORD` |
| 自己构建镜像 | `docker build -f deploy/Dockerfile -t imgnest:local .` |

::: warning 只运行一个实例
不要对同一份数据同时启动多个 ImgNest 容器。
:::

## 接下来

- 调整上传大小上限或数据库连接，看[配置](./configuration)。
- 把图片存到 S3 兼容存储，看[存储与规则](./storage-and-policies)。
- 想改代码，看仓库里的 [`docs/development.md`](https://github.com/biliblihuorong/imgnest/blob/main/docs/development.md)。
