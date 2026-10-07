# 配置

部署参数写在配置文件或环境变量里；站点名、注册开关这类业务设置在管理后台里改，存在数据库中。

## 从哪里读配置

用 Docker 部署时，最简单的做法是在 compose 文件的 `environment` 里写环境变量，仓库里的 `deploy/compose.sqlite.yaml` 和 `deploy/compose.postgres.yaml` 已经接好了常用的几项。

也可以用配置文件。`deploy/config.example.yaml` 是示例，复制一份后用 `--config` 显式加载：

```bash
imgnest serve --config config.yaml
```

`--config` 对所有子命令都有效，`migrate`、`init-admin` 等命令要读同一份配置时也加上它。

三处来源按下面的顺序生效，后面的覆盖前面的：

1. 内置默认值
2. 配置文件
3. `IMGNEST_` 开头的环境变量

环境变量名由配置路径转大写、用下划线连接得到，例如 `database.max_open` 对应 `IMGNEST_DATABASE_MAX_OPEN`。

::: warning 配置出错时服务不会启动
指定的配置文件不存在、YAML 写错、数值不合法或数据库类型不认识时，服务会拒绝启动。报错信息不会回显密钥或整份配置。
:::

## 服务

| 配置项 | 环境变量 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `server.addr` | `IMGNEST_SERVER_ADDR` | `:8080` | 监听地址 |
| `server.trusted_proxies` | `IMGNEST_SERVER_TRUSTED_PROXIES` | 空 | 留空时忽略转发头里的客户端 IP |
| `server.read_header_timeout` | `IMGNEST_SERVER_READ_HEADER_TIMEOUT` | `5s` | 读取请求头的超时 |
| `server.shutdown_timeout` | `IMGNEST_SERVER_SHUTDOWN_TIMEOUT` | `10s` | 停止时等待请求结束的时间 |

## 上传上限

这几项决定了服务最多会用多少资源处理上传。

| 配置项 | 环境变量 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `server.max_upload_mb` | `IMGNEST_SERVER_MAX_UPLOAD_MB` | `20` | 单个文件的大小上限，范围 1 到 20 |
| `server.max_request_mb` | `IMGNEST_SERVER_MAX_REQUEST_MB` | `64` | 整个请求的大小上限，最大 256 |
| `server.upload_concurrency` | `IMGNEST_SERVER_UPLOAD_CONCURRENCY` | `2` | 同时处理的上传请求数 |
| `server.processing_timeout` | `IMGNEST_SERVER_PROCESSING_TIMEOUT` | `5m` | 单次上传的处理时限，最长 30 分钟 |
| `server.max_pixels` | `IMGNEST_SERVER_MAX_PIXELS` | `100000000` | 像素总量上限，动图按全部帧计算 |

一次请求最多带 20 个文件。这些限制在完整读取文件之前就会检查。

## 数据库

| 配置项 | 环境变量 | 默认值 |
| --- | --- | --- |
| `database.driver` | `IMGNEST_DATABASE_DRIVER` | `sqlite` |
| `database.dsn` | `IMGNEST_DATABASE_DSN` | `data/imgnest.db` |
| `database.max_open` / `max_idle` | `IMGNEST_DATABASE_MAX_OPEN` / `MAX_IDLE` | SQLite 为 1 / 1，PostgreSQL 为 25 / 10 |
| `database.max_lifetime` | `IMGNEST_DATABASE_MAX_LIFETIME` | SQLite 为 0，PostgreSQL 为 `5m` |
| `database.busy_timeout` | `IMGNEST_DATABASE_BUSY_TIMEOUT` | `5s` |

### SQLite

适合个人使用，不需要额外的服务。ImgNest 会自动开启 WAL 和外键，并固定使用单个连接。

### PostgreSQL

用户多或上传并发高时换用 PostgreSQL。把驱动设为 `postgres`，连接串通过运行环境的环境变量注入，不要写进会提交的文件：

```ini
IMGNEST_DATABASE_DRIVER=postgres
IMGNEST_DATABASE_DSN=postgres://user:password@host:5432/imgnest
```

仓库里的 `deploy/compose.postgres.yaml` 已经配好了数据库容器和这两个变量，只需要提供 `POSTGRES_PASSWORD`。换库之后要重新创建管理员和存储。

::: tip 迁移是内置的
迁移脚本编译在程序里，重复执行 `migrate` 不会重复写入。已发布的迁移被改动、版本缺失或出现未知版本时会报错。
:::

## 其他

| 配置项 | 环境变量 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `images.thumb_cache` | `IMGNEST_IMAGES_THUMB_CACHE` | `data/thumbs` | 本机缩略图目录，可以整个删掉重建 |
| `security.master_key` | `IMGNEST_SECURITY_MASTER_KEY` | 空 | 加密 S3 凭据用的主密钥，只用本机存储时可以留空 |

使用 S3 存储时必须提供主密钥：32 个随机字节的 base64 编码。它和数据库备份要一起保管，丢了就解不开已保存的存储凭据。

```bash
openssl rand -base64 32
```
