# 从蓝空图床（Lsky Pro）迁移

`imgnest import-lsky` 读取蓝空的 PostgreSQL 数据库，把组、用户、Token、相册映射进 ImgNest 自己的表，再把每张图从旧链接下载回来、**按原路径重新走一遍 ImgNest 的上传流水线**：格式探测、EXIF 入库、原图无损脱敏、WebP、本地 + 云端缩略图、响应式衍生图（规则开启时）和配额，全部和新上传的图片一样。所以导入后的数据库与对象就是 ImgNest 自己产出的结构，不存在「半接管」的旧记录。

## 会导入什么

| 蓝空 | ImgNest | 说明 |
| --- | --- | --- |
| `groups` | `groups` | 同名组直接复用（不改限额）；新建的组带上 `maximum_file_size`、`limit_per_minute`、可用格式（ico/psd 等 ImgNest 不支持的会去掉），默认规则设为 `--policy-id` |
| `users` | `users` | 同邮箱账号直接复用（不改密码/角色/组）；新账号保留 bcrypt 密码（`$2y$` 可直接校验）、管理员身份、注册 IP、创建时间。用户名少于 3 个字时改用邮箱前缀，原名放进显示名 |
| `personal_access_tokens` | `tokens` | 摘要原样保留；编号没被占用时沿用原编号，PicGo/Typora 里的 `id\|secret` Token 无需更换 |
| `albums` | `albums` | 同用户同名相册复用 |
| `images` | `images` + 对象 | 路径（`pathname`）与扩展名不变，`created_at` 保留（年份为 1 的坏时间从 `uniqid` 文件名还原）；公开/私有、上传 IP、原文件名保留 |
| `configs`、`strategies` 的密钥 | 不导入 | 只读策略的公开 `url` 用来下载原图；存储密钥、SMTP 密码不会被读取或打印 |

不会导入：游客上传、已停用用户的图片、ImgNest 不支持的格式（`.ico`、`.psd`），以及「保存后扩展名会变」的图片（文件实际格式和扩展名不符、`.jpeg`、大写扩展名），因为它们的旧链接会失效。命令结束时会逐条列出这些图片，它们仍只在旧存储里。

::: tip 管理员账号怎么对上
账号按邮箱匹配。初始化 ImgNest 管理员时用和蓝空管理员相同的邮箱，导入后蓝空账号的图片和 Token 会直接归到这个账号下，继续用 ImgNest 的密码登录；邮箱不同则会另建一个保留蓝空密码的管理员账号。
:::

## 为什么要新建一个桶

ImgNest 的对象只用「不覆盖」的方式写入并带所有权标记，旧桶里同路径的蓝空原图已经存在，不能原地覆盖。所以图片写进一个**新的桶**（路径与旧桶一一对应），旧桶保持不动，随时可以回退：

1. 导入期间博客仍然读旧桶，什么都不会坏。
2. 导入并抽查完成后，把 CDN / 反代上旧链接的来源（例如 `img.example.com/old-bucket` → 新桶）切过去，再把 ImgNest 里这个存储的 `base_url` 改成旧域名，旧链接就由 ImgNest 管理的对象提供。
3. 跳过的 `.ico` 等文件，切换前用 rclone 之类的工具从旧桶原样复制到新桶，旧链接照样能打开（它们不在 ImgNest 的图片列表里）。

注意：规则的 `scrub_mode` 默认是 `gps`，重新入库后的原图会去掉 GPS 等敏感 EXIF（无损，不重新编码），所以 `md5` 会和蓝空记录不同，这是预期行为。

## 操作步骤（Docker Compose + PostgreSQL）

以下以 `deploy/compose.postgres.yaml` 为例，其他部署方式只要能运行 `imgnest` 二进制即可。

1. **把蓝空导出加载进一个临时库**（导出里有明文密钥，用完删掉）：

   ```bash
   docker compose -f deploy/compose.postgres.yaml exec -T postgres createdb -U imgnest lsky
   docker compose -f deploy/compose.postgres.yaml exec -T postgres psql -q -U imgnest -d lsky < LsyPro.sql
   ```

2. **在 ImgNest 后台准备目标存储和规则**：新建 B2 桶并添加为 S3 存储（需要 `IMGNEST_SECURITY_MASTER_KEY`），再给它建一条规则：WebP 模式选「原图 + WebP」（不能选仅 WebP，否则原图链接会丢）、打开缩略图和「响应式衍生图」。记下规则 ID。

3. **停掉服务后导入**（导入和服务的后台恢复不要同时跑）：

   ```bash
   docker compose -f deploy/compose.postgres.yaml stop imgnest
   export IMGNEST_LSKY_DSN="host=postgres user=imgnest password=$POSTGRES_PASSWORD dbname=lsky sslmode=disable"
   docker compose -f deploy/compose.postgres.yaml run --rm -e IMGNEST_LSKY_DSN imgnest import-lsky --dry-run
   docker compose -f deploy/compose.postgres.yaml run --rm -e IMGNEST_LSKY_DSN imgnest import-lsky --policy-id 2 --limit 5
   # 抽查这 5 张没问题后跑完整
   docker compose -f deploy/compose.postgres.yaml run --rm -e IMGNEST_LSKY_DSN imgnest import-lsky --policy-id 2
   docker compose -f deploy/compose.postgres.yaml start imgnest
   ```

   原图默认从各存储策略的 `url` 下载（例如 `https://img.example.com/old-bucket/<pathname>`），运行命令的机器要能访问它。也可以先把旧桶同步到本地目录，再用 `--source-dir /path/to/dir`（需挂载进容器）。蓝空时间戳按 `--timezone`（默认 `Asia/Shanghai`）解释。

4. **看报告**：命令会输出每张图的 `OK` / `FAIL`，最后列出跳过和失败的图片。失败的修好原因后**直接重跑同一条命令**，已导入的会被跳过，只重试剩下的。

5. **切换旧链接**（见上一节），确认博客图片正常后删除临时库：

   ```bash
   docker compose -f deploy/compose.postgres.yaml exec -T postgres dropdb -U imgnest lsky
   ```

## 之后

- 密码不是 bcrypt 的账号会导入为不可登录，用 `imgnest reset-password --email ...` 重设。
- Token 编号被占用时会换新编号，报告会写明把客户端里 `旧编号|` 改成 `新编号|` 即可。
- 以后再给别的规则打开「响应式衍生图」时，历史图片仍用后台的「响应式图片补生成」补齐；导入时规则已开启的话不需要再回填。
- 目前只支持 PostgreSQL 版蓝空；MySQL 版可以先用 pgloader 之类的工具转成 PostgreSQL 再导入。
