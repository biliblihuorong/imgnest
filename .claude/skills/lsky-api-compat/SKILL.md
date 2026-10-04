---
name: lsky-api-compat
description: Exact contract of the Lsky Pro (蓝空图床) v1 API that ImgNest must stay compatible with — routes, request params, response envelopes, field names and units, auth and error codes — so PicGo, Typora, uPic and scripts keep working. Use whenever adding, changing or testing anything under /api/v1, the token format, or the upload/list/delete responses.
---

# Lsky Pro v1 API compatibility

ImgNest serves `/api/v1/*` with the same shapes as Lsky Pro 2.x so existing
clients work unchanged. Rule: **same fields, same types, same units; extra
fields are allowed, missing or renamed fields are not.**

Contract taken from the Lsky Pro source (`routes/api.php`,
`app/Http/Controllers/Api/V1/*`, `app/Models/Image.php`, `app/Http/Result.php`).
Full request/response samples: [references/contract.md](references/contract.md).

## Envelope

Every response body:

```json
{ "status": true, "message": "success", "data": { } }
```

- `data` is `{}` (an object, never `null` or `[]`) when empty.
- Business failures (wrong password, quota, bad file) → HTTP **200** with
  `"status": false` and a human message. Clients only check `status`.
- HTTP status is used only for: **401** unauthenticated (`"Unauthenticated."`
  or `"Authentication failed."`), **403** API disabled by admin
  (`"管理员未启用 API"`), **429** throttled.

## Auth

- Header `Authorization: Bearer <token>`; clients also send
  `Accept: application/json`.
- Token format is Laravel Sanctum's: `"<id>|<40 random chars>"`. Store
  SHA-256 of the part after `|`; look up by id, compare in constant time.
- `POST /api/v1/upload` is the one route that works without a token: no
  `Authorization` header → guest upload (only if guest upload is enabled).
  An `Authorization` header that fails → 401, never silently fall back to guest.

## Routes

### ImgNest M1 shared authentication decisions

Native and future Lsky endpoints consume the same user/token services.
New native passwords are 12–72 bytes, bcrypt cost 12; login and current-password
verification accept nonempty passwords up to 72 bytes to preserve legacy bcrypt
compatibility. The upper bound is checked before bcrypt comparison because
comparison alone can accept a suffix beyond 72 bytes. Web tokens expire after 24 hours;
API tokens may have no expiry or a future expiry. At `expires_at <= now`
authentication fails. Disabled users cannot authenticate. M1 abilities are
`["*"]`; changing/resetting a password revokes all that user's tokens,
while native logout revokes only the current token. These are shared-service
rules; Lsky routes retain their exact response envelope and HTTP contract.

| Method | Path | Auth | Notes |
| --- | --- | --- | --- |
| POST | `/api/v1/tokens` | none, 3/min | form/json `email`, `password` → `data.token` |
| DELETE | `/api/v1/tokens` | token | revoke all of the user's tokens |
| GET | `/api/v1/strategies` | optional | `data.strategies: [{id, name}]` — map to ImgNest **policies** the user's group (or guest group) may use |
| POST | `/api/v1/upload` | optional | multipart `file` (required), `strategy_id`, `album_id`, `permission` (1 public / 0 private) |
| GET | `/api/v1/images` | token | `page`, `order`, `permission`, `album_id`, `keyword`; 40 per page |
| DELETE | `/api/v1/images/{key}` | token | ImgNest: moves to the recycle bin |
| GET | `/api/v1/albums` | token | `page`, `order`, `keyword`; 40 per page |
| DELETE | `/api/v1/albums/{id}` | token | images stay, `album_id` set to null |
| GET | `/api/v1/profile` | token | user info and capacity |

Filter values:
- images `order`: `newest` (default) | `earliest` | `utmost` (size desc) | `least`
- images `permission`: `all` (default) | `public` | `private`
- albums `order`: `newest` | `earliest` | `most` | `least` (by image count)
- images `album_id` **absent or 0 means "images not in any album"** in Lsky.
  Keep this quirk.

## Field rules that clients depend on

- `size`, `capacity`, `used_capacity` are **kilobytes as floats**
  (`bytes / 1024`), not bytes.
- `key` is the short public id; `pathname` = path + "/" + stored file name;
  `name` = stored file name with extension; `origin_name` = client file name.
- `links.url` is the link clients paste. In ImgNest it follows the policy's
  `link_prefer` (`webp` by default). Always also return `links.origin_url`
  and `links.webp_url` (extra fields).
- `links.html` is HTML-escaped exactly like Lsky:
  `&lt;img src="URL" alt="NAME" title="NAME" /&gt;`.
- `links.thumbnail_url` → the cloud `_thumbs.webp`, or the image URL if none.
- List responses are Laravel paginators (see reference): `current_page`,
  `data`, `last_page`, `per_page`, `total`, plus the url fields.
- Never return EXIF or GPS from any v1 route.

## Checklist for any change under /api/v1

1. Compare the JSON against `references/contract.md` field by field.
2. Run the contract tests (golden JSON files under
   `internal/http/lsky/testdata/`) — add one for any new case.
3. Smoke test with a real client: PicGo + `picgo-plugin-lankong` (or the
   built-in Lsky uploader in uPic), upload one PNG, confirm the returned URL
   opens and is the WebP when `link_prefer = webp`.
