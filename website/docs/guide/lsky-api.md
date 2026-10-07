# 蓝空 API 与 PicGo

ImgNest 的 `/api/v1` 与蓝空图床（Lsky Pro）2.x 的 v1 接口保持一致：路由、字段名、类型和单位都相同，只增加字段，不删除也不改名。为蓝空写的客户端和脚本可以直接接过来。

## 获取 Token

两种方式任选：

- 登录网页，在「账户与令牌」页面创建。明文只在创建时显示一次。
- 调接口，用邮箱和密码换：

```bash
curl -X POST http://127.0.0.1:18080/api/v1/tokens \
  --data-urlencode 'email=you@example.com' \
  --data-urlencode 'password=你的密码'
```

响应里的 `data.token` 就是 Token，格式是 `编号|随机串`。这个接口每分钟最多调用 3 次。

## 接入 PicGo

1. 安装插件 `picgo-plugin-lankong`。
2. 接口地址填 `http://<你的域名>/api/v1/upload`。
3. 鉴权方式选 Bearer，粘贴上一步拿到的 Token。
4. 上传一张图试试，返回的链接应该能直接打开。

uPic 选内置的蓝空图床，Typora 通过 PicGo 上传，填的内容一样。

## 上传

```bash
curl -X POST https://img.example.com/api/v1/upload \
  -H "Authorization: Bearer 1|9f2c…" \
  -H "Accept: application/json" \
  -F file=@photo.png
```

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `file` | 是 | 图片文件 |
| `strategy_id` | 否 | 存储策略 ID，对应 ImgNest 的规则 |
| `album_id` | 否 | 相册 ID |
| `permission` | 否 | 1 为公开，0 为私有 |

```json
{
  "status": true,
  "message": "上传成功",
  "data": {
    "key": "aZ3kP0qL",
    "name": "66ff1b2a41e07.png",
    "pathname": "2026/10/07/66ff1b2a41e07.png",
    "origin_name": "photo.png",
    "size": 2381.42,
    "mimetype": "image/png",
    "extension": "png",
    "md5": "…",
    "sha1": "…",
    "links": {
      "url": "https://img.example.com/2026/10/07/66ff1b2a41e07.webp",
      "html": "&lt;img src=\"…\" alt=\"…\" title=\"…\" /&gt;",
      "bbcode": "[img]…[/img]",
      "markdown": "![…](…)",
      "markdown_with_link": "[![…](…)](…)",
      "thumbnail_url": "https://img.example.com/2026/10/07/66ff1b2a41e07_thumbs.webp",
      "webp_url": "https://img.example.com/2026/10/07/66ff1b2a41e07.webp",
      "origin_url": "https://img.example.com/2026/10/07/66ff1b2a41e07.png"
    }
  }
}
```

需要注意的字段：

- `size` 的单位是 **KB**，带小数，和蓝空一样，不是字节。
- `links.url` 是客户端拿去粘贴的地址，跟随规则的 `link_prefer`，默认是 WebP。
- `links.webp_url` 和 `links.origin_url` 是 ImgNest 多出来的字段，旧客户端会忽略。
- `links.thumbnail_url` 指向云端缩略图。

## 全部接口

| 方法 | 路径 | 是否需要 Token | 说明 |
| --- | --- | --- | --- |
| POST | `/api/v1/tokens` | 否 | 邮箱和密码换 Token |
| DELETE | `/api/v1/tokens` | 是 | 撤销当前用户的全部 Token |
| GET | `/api/v1/strategies` | 可选 | 当前可用的存储策略 |
| POST | `/api/v1/upload` | 可选 | 上传图片 |
| GET | `/api/v1/images` | 是 | 图片列表，每页 40 条 |
| DELETE | `/api/v1/images/{key}` | 是 | 删除图片，移入回收站 |
| GET | `/api/v1/albums` | 是 | 相册列表，每页 40 条 |
| DELETE | `/api/v1/albums/{id}` | 是 | 删除相册，里面的图片保留 |
| GET | `/api/v1/profile` | 是 | 当前用户信息与容量 |

图片列表支持这些筛选参数：

| 参数 | 可选值 |
| --- | --- |
| `page` | 页码 |
| `order` | `newest`（默认）、`earliest`、`utmost`（从大到小）、`least`（从小到大） |
| `permission` | `all`（默认）、`public`、`private` |
| `album_id` | 相册 ID；不传或传 0 表示「不在任何相册里的图片」，这是蓝空原有的行为 |
| `keyword` | 关键词 |

## 响应格式与错误

所有响应都是同一个外壳：

```json
{ "status": true, "message": "success", "data": {} }
```

密码错误、容量不足、文件不合格这类业务失败，HTTP 状态码仍是 **200**，靠 `status: false` 和 `message` 区分。蓝空的客户端只看 `status`，所以这里保持原样。

只有三种情况使用别的状态码：

| 状态码 | 情况 |
| --- | --- |
| 401 | 没带 Token，或 Token 无效 |
| 403 | 管理员关闭了 API |
| 429 | 请求太频繁 |

## 游客上传

管理员在站点设置里开启游客上传后，不带 `Authorization` 头的上传请求会按游客处理，使用游客组的规则，并按 IP 限流。

::: warning 无效的 Token 不会降级成游客
带了 `Authorization` 头但 Token 无效时，返回 401，不会当作游客上传。
:::

## 和蓝空的差别

- 删除图片是移入回收站，不是立即删除，见[隐私与回收站](./privacy-and-trash)。
- 上传响应多了 `links.webp_url` 和 `links.origin_url`。
- v1 接口不返回任何 EXIF 或 GPS 信息。

## 原生接口

ImgNest 自己的界面用的是另一套 `/api/*` 接口，响应外壳是 `{"code", "message", "data"}`，HTTP 状态码按语义返回。两套接口背后是同一套业务逻辑。完整定义见仓库里的 [`docs/openapi.yaml`](https://github.com/biliblihuorong/imgnest/blob/main/docs/openapi.yaml)。
