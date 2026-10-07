# Lsky API and PicGo

ImgNest's `/api/v1` matches the v1 API of Lsky Pro 2.x: the same routes, field names, types and units. Fields are only added, never removed or renamed. Clients and scripts written for Lsky Pro work as they are.

## Get a token

Either way works:

- Sign in on the web and create one under **Account & tokens**. The plain text is shown once, at creation.
- Call the API with your email and password:

```bash
curl -X POST http://127.0.0.1:18080/api/v1/tokens \
  --data-urlencode 'email=you@example.com' \
  --data-urlencode 'password=your-password'
```

`data.token` in the response is the token, in the form `id|random-string`. This route allows 3 calls per minute.

## Connect PicGo

1. Install the `picgo-plugin-lankong` plugin.
2. Set the endpoint to `http://<your-domain>/api/v1/upload`.
3. Choose Bearer authentication and paste the token.
4. Upload a test image. The returned link should open directly.

uPic uses its built-in Lsky uploader, and Typora uploads through PicGo. The values are the same.

## Upload

```bash
curl -X POST https://img.example.com/api/v1/upload \
  -H "Authorization: Bearer 1|9f2c…" \
  -H "Accept: application/json" \
  -F file=@photo.png
```

| Parameter | Required | Meaning |
| --- | --- | --- |
| `file` | Yes | The image file |
| `strategy_id` | No | Storage strategy ID, which maps to an ImgNest policy |
| `album_id` | No | Album ID |
| `permission` | No | 1 for public, 0 for private |

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

Fields worth knowing:

- `size` is in **kilobytes** as a float, as in Lsky Pro. It is not bytes.
- `links.url` is the address clients paste. It follows the policy's `link_prefer`, which defaults to WebP.
- `links.webp_url` and `links.origin_url` are extra fields from ImgNest. Old clients ignore them.
- `links.thumbnail_url` points at the cloud thumbnail.

## All routes

| Method | Path | Token | Meaning |
| --- | --- | --- | --- |
| POST | `/api/v1/tokens` | No | Exchange email and password for a token |
| DELETE | `/api/v1/tokens` | Yes | Revoke all tokens of the current user |
| GET | `/api/v1/strategies` | Optional | Storage strategies available to you |
| POST | `/api/v1/upload` | Optional | Upload an image |
| GET | `/api/v1/images` | Yes | Image list, 40 per page |
| DELETE | `/api/v1/images/{key}` | Yes | Delete an image, moving it to the recycle bin |
| GET | `/api/v1/albums` | Yes | Album list, 40 per page |
| DELETE | `/api/v1/albums/{id}` | Yes | Delete an album; its images are kept |
| GET | `/api/v1/profile` | Yes | Current user and capacity |

The image list accepts these filters:

| Parameter | Values |
| --- | --- |
| `page` | Page number |
| `order` | `newest` (default), `earliest`, `utmost` (largest first), `least` (smallest first) |
| `permission` | `all` (default), `public`, `private` |
| `album_id` | Album ID. Absent or 0 means "images in no album", which is how Lsky Pro behaves |
| `keyword` | Search keyword |

## Responses and errors

Every response has the same envelope:

```json
{ "status": true, "message": "success", "data": {} }
```

Business failures such as a wrong password, a full quota or an unacceptable file still return HTTP **200**. They are told apart by `status: false` and `message`. Lsky Pro clients only check `status`, so this stays unchanged.

Only three cases use another status code:

| Status | Case |
| --- | --- |
| 401 | No token, or an invalid one |
| 403 | An administrator has turned the API off |
| 429 | Too many requests |

## Guest uploads

After an administrator enables guest uploads in the site settings, an upload request without an `Authorization` header is treated as a guest upload. It uses the guest group's policy and is rate limited per IP.

::: warning An invalid token never falls back to guest
A request that carries an `Authorization` header with an invalid token gets 401. It is not treated as a guest upload.
:::

## Differences from Lsky Pro

- Deleting an image moves it to the recycle bin instead of removing it at once. See [Privacy and recycle bin](./privacy-and-trash).
- The upload response adds `links.webp_url` and `links.origin_url`.
- No v1 route returns EXIF or GPS data.

## Native API

ImgNest's own interface uses a separate `/api/*` API with a `{"code", "message", "data"}` envelope and semantic HTTP status codes. Both APIs run on the same business logic. The full definition is in [`docs/openapi.yaml`](https://github.com/biliblihuorong/imgnest/blob/main/docs/openapi.yaml) in the repository.
