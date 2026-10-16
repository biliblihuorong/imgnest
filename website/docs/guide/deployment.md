# 部署

ImgNest 有两种部署方式：

- **Docker 部署**（推荐）：官方镜像 `ghcr.io/biliblihuorong/imgnest`，支持 linux/amd64 和 linux/arm64，数据库可选 SQLite 或 PostgreSQL。
- **二进制部署**：自己编译出单个 `imgnest` 可执行文件（前端已嵌入），用 systemd 等方式运行。目前不提供预编译的二进制下载。

两种方式用的是同一套程序和同一套数据库迁移，之后可以互相切换。

## 选哪个数据库

| | SQLite | PostgreSQL |
| --- | --- | --- |
| 适合 | 个人自用、小站点 | 多用户、图片较多 |
| 额外服务 | 不需要，数据库是数据卷里的一个文件 | 需要一个 PostgreSQL 容器或实例 |
| 备份 | 备份 `imgnest-data` 数据卷 | 备份 `imgnest-data` 数据卷 + `pg_dump` |

同一个镜像同时支持两种数据库，由环境变量 `IMGNEST_DATABASE_DRIVER` 和 `IMGNEST_DATABASE_DSN` 在运行时选择。

## Docker 部署

准备好 Docker 和 Compose 插件，新建一个目录（例如 `/opt/imgnest`），把下面任选其一的内容保存为 `compose.yaml`。仓库里的 `deploy/compose.sqlite.yaml` 和 `deploy/compose.postgres.yaml` 与这里一致。

::: code-group

```yaml [SQLite]
# 单容器 + SQLite，适合个人或小规模使用
name: imgnest
services:
  imgnest:
    image: ghcr.io/biliblihuorong/imgnest:${IMGNEST_TAG:-latest}
    restart: unless-stopped
    ports:
      - "${IMGNEST_PORT:-8080}:8080"
    environment:
      # 只有使用 S3 兼容存储时需要：32 字节随机数的 base64
      IMGNEST_SECURITY_MASTER_KEY: ${IMGNEST_SECURITY_MASTER_KEY:-}
      # 允许设置客户端 IP 的反向代理地址，逗号分隔
      IMGNEST_SERVER_TRUSTED_PROXIES: ${IMGNEST_SERVER_TRUSTED_PROXIES:-}
    volumes:
      # 数据库、本机图片和缩略图缓存
      - imgnest-data:/app/data
volumes:
  imgnest-data:
```

```yaml [PostgreSQL]
# ImgNest + PostgreSQL，适合多用户
# 先在环境变量或同目录的 .env 里设置 POSTGRES_PASSWORD
name: imgnest
services:
  imgnest:
    image: ghcr.io/biliblihuorong/imgnest:${IMGNEST_TAG:-latest}
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    ports:
      - "${IMGNEST_PORT:-8080}:8080"
    environment:
      IMGNEST_DATABASE_DRIVER: postgres
      IMGNEST_DATABASE_DSN: host=postgres user=imgnest password=${POSTGRES_PASSWORD:?set POSTGRES_PASSWORD} dbname=imgnest sslmode=disable
      # 只有使用 S3 兼容存储时需要：32 字节随机数的 base64
      IMGNEST_SECURITY_MASTER_KEY: ${IMGNEST_SECURITY_MASTER_KEY:-}
      # 允许设置客户端 IP 的反向代理地址，逗号分隔
      IMGNEST_SERVER_TRUSTED_PROXIES: ${IMGNEST_SERVER_TRUSTED_PROXIES:-}
    volumes:
      # 本机图片和缩略图缓存
      - imgnest-data:/app/data
  postgres:
    image: postgres:18.6-bookworm
    restart: unless-stopped
    environment:
      POSTGRES_USER: imgnest
      POSTGRES_DB: imgnest
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?set POSTGRES_PASSWORD}
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U imgnest -d imgnest"]
      interval: 5s
      timeout: 3s
      retries: 30
    volumes:
      - postgres-data:/var/lib/postgresql
volumes:
  imgnest-data:
  postgres-data:
```

:::

在同一目录新建 `.env`，按需填写（PostgreSQL 必须填 `POSTGRES_PASSWORD`，不要提交到 Git）：

```ini
IMGNEST_TAG=0.0.3
IMGNEST_PORT=8080
# PostgreSQL 版必填
POSTGRES_PASSWORD=换成一个长随机密码
# 用 S3 / B2 / R2 / COS 存储时填写，生成方式：openssl rand -base64 32
IMGNEST_SECURITY_MASTER_KEY=
```

启动：

```bash
docker compose up -d
```

容器启动时会自动执行数据库迁移。之后按[快速开始](./getting-started)创建管理员和本机存储，命令里的 `-f deploy/compose.sqlite.yaml` 去掉即可（当前目录的 `compose.yaml` 会被自动使用）：

```bash
read -rsp '管理员密码: ' password; echo
printf '%s' "$password" | docker compose exec -T imgnest \
  imgnest init-admin --username admin --email admin@example.com
unset password
docker compose exec imgnest imgnest init-local --base-url https://img.example.com
```

::: warning 务必备份 IMGNEST_SECURITY_MASTER_KEY
S3 存储的密钥用它加密保存在数据库里。丢了它，已添加的 S3 存储就无法解密，只能重新填写密钥。
:::

### 升级

把 `.env` 里的 `IMGNEST_TAG` 改成新版本后执行：

```bash
docker compose pull
docker compose up -d
```

新容器启动时会自动迁移数据库。升级前建议先备份数据卷（PostgreSQL 版再加一份 `pg_dump`）。

## 二进制部署

二进制是一个文件，前端界面已经嵌在里面。它在运行时动态链接 libvips，所以**运行的机器上必须装有 libvips 8.18.x**（版本需与编译时一致）。

### 1. 编译

需要 Git 和 Docker。在仓库里切到要部署的版本，用开发镜像编译（Go、Node、pnpm 和 libvips 都在镜像里）：

```bash
git clone https://github.com/biliblihuorong/imgnest
cd imgnest
git checkout v0.0.3
make release   # 先构建前端，再输出 bin/imgnest
```

也可以不用 Docker，在本机装好 Go 1.27.1、Node 24、pnpm 12.9.1 和 libvips 8.18.x（含开发头文件）后编译：

```bash
cd web-vben && pnpm install --frozen-lockfile && pnpm build && cd ..
go build -trimpath -o bin/imgnest ./cmd/imgnest
```

### 2. 配置

把 `bin/imgnest` 复制到服务器，例如 `/opt/imgnest/imgnest`，再把仓库里的 `deploy/config.example.yaml` 复制为 `/opt/imgnest/config.yaml`。配置里的相对路径（`data/imgnest.db`、`data/thumbs`，以及本机存储默认的 `data/images`）以运行目录为准。不放配置文件也能运行，这时全部使用默认值。

- **SQLite**：示例配置直接可用，数据库在 `data/imgnest.db`。
- **PostgreSQL**：用环境变量选择，密码不要写进配置文件：

  ```bash
  IMGNEST_DATABASE_DRIVER=postgres
  IMGNEST_DATABASE_DSN="host=127.0.0.1 user=imgnest password=... dbname=imgnest sslmode=disable"
  ```

  并删掉配置里 SQLite 专用的 `max_open: 1`、`max_idle: 1`，让 PostgreSQL 使用默认连接池。

所有配置项见[配置](./configuration)。

### 3. 初始化并运行

二进制不会自动迁移，首次启动和每次升级前都要先执行 `migrate`：

```bash
cd /opt/imgnest
mkdir -p data
./imgnest --config config.yaml migrate
read -rsp '管理员密码: ' password; echo
printf '%s' "$password" | ./imgnest --config config.yaml init-admin --username admin --email admin@example.com
unset password
./imgnest --config config.yaml init-local --base-url https://img.example.com
./imgnest --config config.yaml serve
```

用 systemd 常驻运行，保存为 `/etc/systemd/system/imgnest.service`：

```ini
[Unit]
Description=ImgNest
After=network-online.target
Wants=network-online.target

[Service]
User=imgnest
WorkingDirectory=/opt/imgnest
# PostgreSQL、S3 主密钥等放在这个文件里，权限设为 600
EnvironmentFile=-/opt/imgnest/imgnest.env
ExecStartPre=/opt/imgnest/imgnest --config config.yaml migrate
ExecStart=/opt/imgnest/imgnest --config config.yaml serve
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

```bash
sudo useradd --system --home /opt/imgnest imgnest
sudo chown -R imgnest:imgnest /opt/imgnest
sudo systemctl daemon-reload
sudo systemctl enable --now imgnest
```

升级时替换 `/opt/imgnest/imgnest` 后 `sudo systemctl restart imgnest`，`ExecStartPre` 会先迁移数据库。

## 反向代理

不管哪种方式，正式使用时都建议在前面放 Nginx、Caddy 等反向代理处理 HTTPS，并把代理地址填进 `IMGNEST_SERVER_TRUSTED_PROXIES`，否则记录的上传 IP 都会是代理的地址。上传大小上限由 ImgNest 控制，代理的请求体上限要不小于它（Nginx 的 `client_max_body_size`）。

::: warning 只运行一个实例
不要对同一份数据同时启动多个 ImgNest，也不要让 Docker 版和二进制版同时连同一个数据库。
:::
