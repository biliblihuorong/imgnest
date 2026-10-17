# Responsive images API

When "Responsive variants" is on in a policy, still images get WebP versions 400, 800 and 1600 pixels wide (only the widths narrower than the original) besides the original, WebP and thumbnail. A blog references them with `srcset`, so phones load the small one and large screens the big one. See [Deployment](./deployment#enabling-responsive-images) to turn it on and [Storage and policies](./storage-and-policies#responsive-image-versions) for file names.

There are two ways to get the sizes:

| Way | When to use it |
| --- | --- |
| From the upload response | Every upload API (native, v2, Lsky v1) returns `variants`. Write the `srcset` when you publish, and the page makes no extra request |
| Look up by link | The post only has original links (older posts, other uploaders). Call the public `/api/variants` endpoint |

## Lookup endpoint

```
GET  /api/variants?url=<link>&url=<link>…
POST /api/variants        {"urls": ["<link>", …]}
```

- Public and read-only; no token. 1–50 links per request, each up to 2048 bytes.
- Original, `.webp`, `_thumbs.webp` and variant links all resolve to the same image. Unknown links and images in the recycle bin return `found: false`.
- ImgNest matches links against the storages' public base URLs in its own database and **never fetches the links you send**.
- 60 requests per minute per client address (`images.variant_lookup_per_minute`); above that you get 429.
- To call it from a browser on another site, add the blog's origin to `images.variant_cors_origins` (see [Configuration](./configuration)). Server-side rendering and build scripts are not affected.

Response:

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

`items` keeps the request order. `variants` runs from narrowest to widest and is an empty array until versions exist. The endpoint never returns EXIF, the uploader or other internals.

## Use GET so a CDN can cache it

A successful GET response carries `Cache-Control: public, max-age=300, s-maxage=3600, stale-while-revalidate=60` and an `ETag`, so CDNs and browsers can cache it, and a request with `If-None-Match` gets 304. `images.variant_cdn_max_age` sets `s-maxage` (default `1h`; `0s` turns the CDN-only lifetime off). CDNs do not cache POST, so **prefer GET when you have few links**, and keep the same links in the same order so the CDN hits the same entry.

The server also caches lookups for 5 minutes (in Redis when `cache.redis_url` is set, otherwise in process memory) and drops an entry as soon as the image is trashed, restored or gets new variants.

## Examples

### curl

```bash
curl -s 'https://img.example.com/api/variants?url=https://img.example.com/2026/10/66ff1b2a3c4d5.jpg'

# Several images at once
curl -s https://img.example.com/api/variants \
  -H 'Content-Type: application/json' \
  -d '{"urls":["https://img.example.com/2026/10/a.jpg","https://img.example.com/2026/10/b.png"]}'
```

### JavaScript (browser or Node 18+)

```js
const IMGNEST = "https://img.example.com"; // the ImgNest site, not the storage base URL

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
  console.log(url, variants.length ? srcset(variants) : "(no variants, use the original)");
}
```

URLs have a length limit, so split many links into batches of at most 50, and switch to POST when links are very long.

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

### In HTML

Write the `variants` you looked up (or got at upload) as `srcset`, and pick a version no wider than 800 as `src` for old browsers:

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

For a complete page that swaps original links in the browser (with a lightbox), see [`docs/examples/responsive-images.html`](https://github.com/biliblihuorong/imgnest/blob/main/docs/examples/responsive-images.html) in the repository.
