# 存储与规则

**存储**决定图片放在哪里，**规则**决定图片叫什么名字、要不要转 WebP、怎么处理隐私信息。一条规则对应一个存储，用户组绑定规则后，组里的用户上传时就按它执行。

## 存储

### 本机存储

`init-local` 会创建一个本机存储和一条默认规则：

```bash
docker compose -f deploy/compose.sqlite.yaml exec imgnest \
  imgnest init-local --base-url https://img.example.com
```

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `--base-url` | `http://localhost:8080` | 对外访问的域名，程序会在后面加上 `/i/<存储 ID>` |
| `--root` | `data/images` | 文件存放目录，镜像里位于数据卷 `/app/data` 下 |
| `--name` | `local` | 存储名称 |

::: warning 不要直接把存储目录当静态目录发布
本机存储目录里的 `.jpg`、`.webp` 是 ImgNest 的内部封装格式，不是普通图片文件，只能通过程序访问。备份时保留整个目录。
:::

### S3 兼容存储

先[设置主密钥](./configuration#其他)，再把存储配置写进一个不提交的文件：

```json
{
  "name": "cloud",
  "driver": "s3",
  "base_url": "https://images.example.com",
  "config": {
    "endpoint": "https://s3.example.com",
    "region": "us-east-1",
    "bucket": "your-bucket",
    "access_key_id": "YOUR_ACCESS_KEY",
    "secret_access_key": "YOUR_SECRET",
    "use_path_style": true
  }
}
```

通过标准输入创建存储，再为它建一条规则：

```bash
docker compose -f deploy/compose.sqlite.yaml exec -T imgnest \
  imgnest init-storage < private-storage.json

docker compose -f deploy/compose.sqlite.yaml exec imgnest \
  imgnest init-policy --storage-id 2 --name cloud
```

`--storage-id` 填第一条命令输出的 ID。创建时会真实测试连接：写入、复制、清理各做一次，不支持所需能力的服务会被拒绝。凭据加密后存入数据库，之后任何接口都不会再返回它。

也可以在管理后台的「存储管理」里创建和测试存储。

## 一张图对应的文件

规则先算出一个不带后缀的路径，再派生出几个文件：

| 文件 | 位置 | 地址示例 |
| --- | --- | --- |
| 原图 | 存储里的 `{path}.{ext}` | `https://img.example.com/26/10/66ff1b2a3c4d5.png` |
| WebP | 存储里的 `{path}.webp` | `https://img.example.com/26/10/66ff1b2a3c4d5.webp` |
| 云端缩略图 | 存储里的 `{path}_thumbs.webp` | `https://img.example.com/26/10/66ff1b2a3c4d5_thumbs.webp` |
| 本机缩略图 | 本机的 `data/thumbs/` | `/t/{key}.webp`，只给后台列表和预览用 |
| 响应式版本（可选） | 存储里的 `{path}_w800_v1.webp` 等 | `https://img.example.com/26/10/66ff1b2a3c4d5_w800_v1.webp` |

访问地址不存进数据库，每次用存储的 `base_url` 实时拼出来。换域名时只改存储配置，所有链接跟着变。

`_thumbs` 和 `_w数字_v数字`（如 `_w800_v1`）是保留后缀，文件名不能以它们结尾。

## 路径模板

规则里有两个模板：目录模板和文件名模板。默认值沿用蓝空的写法，目录是 <code v-pre>{Y}/{m}/{d}</code>，文件名是 <code v-pre>{uniqid}</code>。

<div v-pre>

| 变量 | 含义 | 示例 |
| --- | --- | --- |
| `{Y}` / `{y}` | 4 位 / 2 位年 | 2026 / 26 |
| `{m}` / `{d}` | 月 / 日，补零 | 10 / 04 |
| `{H}` `{i}` `{s}` | 时 / 分 / 秒 | 14 / 05 / 09 |
| `{timestamp}` | Unix 秒 | 1791091509 |
| `{uniqid}` | 13 位按时间排序的 ID | 66ff1b2a3c4d5 |
| `{md5}` / `{md5-16}` | 文件内容的 MD5 | 9e107d9d… |
| `{sha1}` | 文件内容的 SHA-1 | 2fd4e1c6… |
| `{str-random-16}` / `{str-random-10}` | 随机字母数字 | aZ3kP0qL9xW2bN7c |
| `{rand:N}` | N 位随机小写字母数字，N 为 1 到 64 | `{rand:6}` 得到 k3x9a0 |
| `{hash:N}` | 内容 MD5 的前 N 位，用来打散目录 | `{hash:2}` 得到 9e |
| `{uuid}` | UUID v4 | 1b4e28ba-… |
| `{filename}` | 原文件名，去掉后缀并清洗 | my-photo |
| `{uid}` | 上传者 ID，游客为 0 | 3 |

</div>

::: tip 和蓝空的一处不同
蓝空的 <code v-pre>{md5}</code> 是随机值，ImgNest 改成了文件内容的哈希，可以用来去重。
:::

常用的组合：

<div v-pre>

| 场景 | 目录模板 | 文件名模板 |
| --- | --- | --- |
| 个人博客 | `{y}/{m}` | `{uniqid}` |
| 图片很多，避免单个目录过大 | `{Y}/{m}/{hash:2}` | `{md5-16}` |
| 保留原文件名 | `{Y}/{m}/{d}` | `{filename}` |

</div>

### 生成路径时的规则

- 后缀由图片的真实格式决定，不看上传时的文件名；统一小写，`jpeg` 写成 `jpg`。
- 空白和 `? # % & \ : * " < > |` 会被替换成 `-`，中文保留。
- 路径重名时，带随机变量的模板会重新生成，最多 5 次；不带随机变量的模板会在文件名后面加 `-1`、`-2`。
- 保存规则时会检查模板，写了不认识的变量会报错。

## WebP 与缩略图

| 模式 `webp_mode` | 保存的内容 | 适合 |
| --- | --- | --- |
| `both`（默认） | 原图和 WebP | 要保留原图，又想让链接更轻 |
| `webp_only` | 只保存 WebP | 省空间，纯博客配图 |
| `none` | 只保存原图 | 摄影原片、需要原样存档 |

| 规则字段 | 默认值 | 说明 |
| --- | --- | --- |
| `webp_quality` | 80 | WebP 质量，1 到 100 |
| `webp_lossless` | false | 无损 WebP，适合截图和图标 |
| `max_width` / `max_height` | 0（不限制） | 只缩小 WebP 版本，原图不动 |
| `thumb_enabled` / `thumb_size` | true / 400 | 缩略图长边的像素数 |
| `link_prefer` | `webp` | 接口返回的主链接用哪个版本 |
| `scrub_mode` | `gps` | 隐私清理档位，见[隐私与回收站](./privacy-and-trash) |

各种格式的处理方式：

- JPEG、PNG、BMP、TIFF、HEIC、AVIF 按规则转换，转换前会按 EXIF 方向自动旋转。
- GIF 的全部帧会转成动态 WebP。
- 上传的本身就是 WebP 时不会重复转换。
- SVG 默认拒绝上传，因为它可以内嵌脚本。

格式判断只看文件头和 libvips 的探测结果，后缀和 `Content-Type` 都不作数。

## 响应式图片版本

在「规则管理 → 编辑规则 → 转码」里打开「响应式衍生图」后，新上传的静态图会额外生成宽 400、800、1600 像素的 WebP，方便博客用 `srcset` 按屏幕挑选合适大小。CDN 只需要缓存，不用帮忙裁图。

- 只缩小不放大：原图比目标宽度窄时跳过这一档；主 WebP 宽度正好是 1600 时直接复用它。
- 动图不生成，不会变成静态首帧。原图、WebP、缩略图和原有链接都不受影响。
- 文件名带处理版本（`_v1`），生成后从不覆盖，可以放心长期缓存。
- 衍生文件计入容量，删除、恢复和彻底删除时与原图一起处理。
- 上传接口和蓝空 v1 接口都会多返回 `variants: [{url, width, height, bytes, mime}]`，其他字段不变；没有版本时是空数组。
- 已有图片：在规则管理页底部的「响应式图片补生成」卡片里开始，可以随时暂停、继续，重启后接着跑。还没补到的图返回空数组，链接照常可用。
- 只知道图片链接时，可以调用公开接口 `GET /api/variants?url=图片链接`（一次最多 50 个）。它只认本站存储的链接，不会去抓外部网址，也不返回 EXIF 或账户信息。博客在浏览器里直接调用时，需要在配置里把博客域名加进 `images.variant_cors_origins`。完整示例见仓库里的 `docs/examples/responsive-images.html`。
