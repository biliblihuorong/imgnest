# 响应式图片 API

规则里打开「响应式衍生图」后，静态图除了原图、WebP 和缩略图，还会多生成宽 400、800、1600 像素的 WebP（比原图窄的才生成）。博客用 `srcset` 引用它们，手机加载小图，大屏加载大图。开启步骤见[部署](./deployment.md#开启响应式图片)，文件命名见[存储与规则](./storage-and-policies.md#响应式图片版本)。

拿到各个尺寸有两种办法：

| 办法 | 什么时候用 |
| --- | --- |
| 上传时直接拿 | 上传接口（原生、v2、蓝空 v1）的响应都带 `variants`，写文章时直接生成 `srcset`，页面加载时零请求 |
| 按链接查询 | 文章里只有原图链接（历史文章、别的工具上传的），调用公开接口 `/api/variants` 反查 |

## 从哪里开启和使用

- 管理员：站点管理 → 规则管理 → 对应规则的「编辑」→「转码」→ 打开「响应式衍生图」并保存。历史图片在同页底部「响应式图片补生成」点「开始补生成」。
- 博客作者：从上传接口响应的 `variants` 取各尺寸，或用下文接口查询，再写入博客 HTML 的 `srcset`。图片列表没有一键修改博客的按钮，普通 Markdown 图片语法也不会自动加 `srcset`。
- 开启后上传一张足够宽的静态测试图，确认 `variants` 包含比原图窄的尺寸；小图或动图没有衍生尺寸是正常情况。

## 查询接口

```
GET  /api/variants?url=<链接>&url=<链接>…
POST /api/variants        {"urls": ["<链接>", …]}
```

- 公开只读，不需要 Token。每批 1–50 个链接，单个不超过 2048 字节。
- 原图、`.webp`、`_thumbs.webp` 和衍生图链接都能查到同一张图；不认识的链接、回收站里的图返回 `found: false`。
- ImgNest 只在自己的库里按存储的访问地址匹配，**从不去抓取你给的链接**。
- 每个客户端地址每分钟 60 次（`images.variant_lookup_per_minute`），超出返回 429。
- 浏览器跨域调用前，要把博客域名填进 `images.variant_cors_origins`（见[配置](./configuration.md)）。服务端渲染、构建脚本调用不受这个限制。

响应：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [
      {
        "url": "https://img.example.com/2026/10/66ff1b2a3c4d5.jpg",
        "found": true,
        "variants": [
          { "url": "https://img.example.com/2026/10/66ff1b2a3c4d5_w400_v1.webp", "width": 400, "height": 225, "bytes": 18234, "mime": "image/webp" },
          { "url": "https://img.example.com/2026/10/66ff1b2a3c4d5_w800_v1.webp", "width": 800, "height": 450, "bytes": 52110, "mime": "image/webp" }
        ]
      }
    ]
  }
}
```

`items` 的顺序与请求一致；`variants` 从窄到宽排列，还没生成时是空数组。接口不返回 EXIF、上传者或任何内部信息。

## 用 GET 才能被 CDN 缓存

成功的 GET 响应带 `Cache-Control: public, max-age=300, s-maxage=3600, stale-while-revalidate=60` 和 `ETag`，CDN 和浏览器都能缓存，带 `If-None-Match` 再来会得到 304。`s-maxage` 由 `images.variant_cdn_max_age` 控制（默认 `1h`，`0s` 关闭 CDN 专用缓存）。POST 不会被 CDN 缓存，所以**链接不多时优先用 GET**，并让同一组链接保持相同的顺序，CDN 才能命中同一份缓存。

服务端自己也会缓存查询结果 5 分钟（配了 `cache.redis_url` 用 Redis，否则用进程内存），图片进回收站、恢复或补生成时立即失效。

## 示例

### curl

```bash
curl -s 'https://img.example.com/api/variants?url=https://img.example.com/2026/10/66ff1b2a3c4d5.jpg'

# 一次查多张
curl -s https://img.example.com/api/variants \
  -H 'Content-Type: application/json' \
  -d '{"urls":["https://img.example.com/2026/10/a.jpg","https://img.example.com/2026/10/b.png"]}'
```

### JavaScript（浏览器或 Node 18+）

```js
const IMGNEST = "https://img.example.com"; // ImgNest 站点地址，不是存储的访问地址

async function lookupVariants(urls) {
  const query = new URLSearchParams(urls.map((url) => ["url", url]));
  const response = await fetch(`${IMGNEST}/api/variants?${query}`);
  const body = await response.json();
  if (!response.ok || body.code !== 0) throw new Error(body.message);
  return new Map(body.data.items.map((item) => [item.url, item.variants]));
}

function srcset(variants) {
  return variants.map((v) => `${v.url} ${v.width}w`).join(", ");
}

const found = await lookupVariants(["https://img.example.com/2026/10/66ff1b2a3c4d5.jpg"]);
for (const [url, variants] of found) {
  console.log(url, variants.length ? srcset(variants) : "（没有衍生图，用原图）");
}
```

GET 的地址有长度上限，链接很多时分批，每批不超过 50 个；链接特别长时改用 POST。

### Python

```python
import requests

IMGNEST = "https://img.example.com"

def lookup_variants(urls):
    resp = requests.get(f"{IMGNEST}/api/variants", params=[("url", u) for u in urls], timeout=10)
    resp.raise_for_status()
    body = resp.json()
    if body["code"] != 0:
        raise RuntimeError(body["message"])
    return {item["url"]: item["variants"] for item in body["data"]["items"]}

for url, variants in lookup_variants(["https://img.example.com/2026/10/66ff1b2a3c4d5.jpg"]).items():
    print(url, ", ".join(f'{v["url"]} {v["width"]}w' for v in variants))
```

### 写进 HTML

把查到（或上传时拿到）的 `variants` 写成 `srcset`，`src` 选一个不超过 800 宽的版本给旧浏览器：

```html
<img
  src="https://img.example.com/2026/10/66ff1b2a3c4d5_w800_v1.webp"
  srcset="
    https://img.example.com/2026/10/66ff1b2a3c4d5_w400_v1.webp   400w,
    https://img.example.com/2026/10/66ff1b2a3c4d5_w800_v1.webp   800w,
    https://img.example.com/2026/10/66ff1b2a3c4d5_w1600_v1.webp 1600w
  "
  sizes="(max-width: 832px) calc(100vw - 32px), 800px"
  width="1600"
  height="900"
  loading="lazy"
  decoding="async"
  alt=""
/>
```

只有原图链接、想在浏览器里自动替换的完整页面（含灯箱）见仓库里的 [`docs/examples/responsive-images.html`](https://github.com/biliblihuorong/imgnest/blob/main/docs/examples/responsive-images.html)。
