# 快速开始

这一页带你在本机用 SQLite 把 ImgNest 跑起来，并上传第一张图。

## 准备

- Docker Desktop，使用 Linux 引擎
- 仓库源码：`git clone https://github.com/biliblihuorong/imgnest`

所有命令都在仓库根目录执行。开发镜像里已经固定了 Go 和 libvips 的版本，本机不需要另外安装。

## 1. 构建镜像并初始化数据库

```bash
docker compose -f deploy/compose.dev.yaml build dev
docker compose -f deploy/compose.dev.yaml run --rm dev go run ./cmd/imgnest migrate
```

数据库没有初始化时，服务会拒绝启动，所以 `migrate` 必须先执行。数据默认存在 `data/imgnest.db`。

## 2. 创建管理员

密码从标准输入读取，不会出现在命令参数、文件或日志里。密码长度为 12 到 72 个字节。

::: code-group

```powershell [PowerShell]
$password = Read-Host '管理员密码' -AsSecureString
$plain = [System.Net.NetworkCredential]::new('', $password).Password
$plain | docker compose -f deploy/compose.dev.yaml run --rm -T dev `
  go run ./cmd/imgnest init-admin --username admin --email admin@example.com
Remove-Variable plain, password
```

```bash [Bash]
read -rsp '管理员密码: ' password; echo
printf '%s' "$password" | docker compose -f deploy/compose.dev.yaml run --rm -T dev \
  go run ./cmd/imgnest init-admin --username admin --email admin@example.com
unset password
```

:::

把用户名和邮箱换成你自己的。重复执行不会覆盖已有的管理员。

## 3. 创建本机存储

```bash
docker compose -f deploy/compose.dev.yaml run --rm dev \
  go run ./cmd/imgnest init-local --base-url http://localhost:18080
```

这条命令会创建一个本机存储和一条默认上传规则，并绑定到默认用户组。

## 4. 启动服务

```bash
docker compose -f deploy/compose.dev.yaml run --rm --service-ports dev go run ./cmd/imgnest serve
```

服务监听 `127.0.0.1:18080`。打开 <http://127.0.0.1:18080>，用刚才的邮箱和密码登录。

检查服务是否就绪：

```bash
curl http://127.0.0.1:18080/healthz
```

按 `Ctrl+C` 停止服务。

## 5. 上传第一张图

登录后进入「上传图片」，拖一张图进去。上传完成后你会拿到原图、WebP 和缩略图三个地址，默认复制的是 WebP 地址。

::: tip 注册默认关闭
新装好的站点不开放注册。需要的话，管理员可以在「站点管理 → 站点设置」里打开。
:::

## 使用新版界面

默认构建嵌入的是经典前端。想用带新版布局的界面，构建另一个二进制：

```bash
make release-vben
```

产物是 `bin/imgnest-vben`。两套界面的区别见[界面与主题](./frontends)。

## 接下来

- 调整端口、上传大小上限或换用 PostgreSQL，看[配置](./configuration)。
- 把图片存到 S3 兼容存储，看[存储与规则](./storage-and-policies)。
