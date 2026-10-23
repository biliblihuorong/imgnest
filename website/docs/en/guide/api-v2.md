# Native API v2

`/api/v2` is ImgNest's own versioned API for scripts and upload clients. It runs alongside the [Lsky-compatible `/api/v1`](./lsky-api.md): existing PicGo / Typora / uPic setups keep working while new clients move to v2.

## Where to start

This is a scripting API, with no separate v2 page or button. Sign in to your site, create and copy an API Token in Account → API tokens (`/account/tokens`), then call `/api/v2` below. See [Getting a Token](./lsky-api.md#get-a-token) for console steps. Use Upload images for ordinary browser uploads.

## How it differs from v1

- **Sizes are bytes**: v1 uses KB floats for Lsky compatibility; v2 is bytes throughout.
- **Native envelope**: `{"code": 0, "message": "ok", "data": ...}`; `code ≠ 0` is a failure and `data.reason` may carry a hint code (semantics in the OpenAPI file).
- **Stable**: v2 only ever gains fields and routes; nothing is renamed or removed.
- **Same behavior as the console**: every v2 route is served by the same handler and service as the matching `/api` route, with the validation and error codes documented in the OpenAPI file.
- Like v1, **no v2 response ever contains EXIF / GPS**.

## Authentication

`Authorization: Bearer <token>`, created on the Account → API tokens page; the plaintext is shown once at creation. The administrator's API switch (`api_enabled`) governs v1 and v2 together: while off, v2 answers `403` with `code: 20003` and `data.reason: "api_disabled"`. API tokens obey the switch on the console's `/api` routes too; the admin console (`/api/admin/*`), token management (`/api/tokens`) and password change need a signed-in web session, and an API token gets `403` with `data.reason: "api_token_forbidden"`.

## Routes

| Method | Path | Same as | Notes |
| --- | --- | --- | --- |
| GET | `/api/v2/profile` | `GET /api/auth/me` | Current account and usage |
| GET | `/api/v2/policies` | `GET /api/policies` | Upload rules this account may use |
| POST | `/api/v2/upload` | `POST /api/upload` | multipart `file`, optional `policy_id`, `album_id`, `is_public` |
| GET | `/api/v2/images` | `GET /api/images` | Paging, filters and [unified search](./search.md) (`qv=1&q=...`) |
| GET | `/api/v2/images/{id}` | `GET /api/images/{id}` | |
| PATCH | `/api/v2/images/{id}` | `PATCH /api/images/{id}` | `is_public` and/or `name` (display name; links never change) |
| DELETE | `/api/v2/images/{id}` | `DELETE /api/images/{id}` | Moves to the recycle bin |
| POST | `/api/v2/images/batch` | `POST /api/images/batch` | `delete` / `permission` / `album` (move) |
| GET | `/api/v2/albums/{id}/images` | `GET /api/albums/{id}/images` | Search within one album |
| GET | `/api/v2/trash` | `GET /api/trash` | |
| POST | `/api/v2/trash/restore` | `POST /api/trash/restore` | |
| GET | `/api/v2/albums` | `GET /api/albums` | |
| POST | `/api/v2/albums` | `POST /api/albums` | |
| PATCH | `/api/v2/albums/{id}` | `PATCH /api/albums/{id}` | |
| DELETE | `/api/v2/albums/{id}` | `DELETE /api/albums/{id}` | Images stay, leaving the album |

## Examples

```bash
# Upload
curl -H "Authorization: Bearer $TOKEN" -F file=@photo.jpg https://img.example.com/api/v2/upload

# Search your images: jpg since October 2026, largest first
curl -H "Authorization: Bearer $TOKEN" \
  "https://img.example.com/api/v2/images?qv=1&q=format%3Ajpg%20after%3A2026-10-01%20sort%3Asize-desc&tz=Asia%2FShanghai&page=1&size=20"
```

Full field definitions live in `docs/openapi.yaml` in the repository; the frontend's types are generated from it too.
