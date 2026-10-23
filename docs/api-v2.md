# ImgNest API v2

`/api/v2` is ImgNest's own versioned API for scripts and upload clients. It runs
alongside the Lsky Pro compatible `/api/v1`, which stays unchanged, so existing
PicGo / Typora / uPic setups keep working while clients move to v2 over time.

- **Same behavior as the console.** Every v2 route is served by the same handler
  and service as the matching `/api` route listed below, so request bodies,
  validation, responses and error codes are the ones documented for that route in
  `docs/openapi.yaml`.
- **Envelope.** `{"code": 0, "message": "ok", "data": ...}`; `code` ≠ 0 is a
  failure and `data.reason` may carry a hint code (see `ErrorReason` in the
  OpenAPI file). Sizes are bytes, not Lsky's kilobytes.
- **Stable.** v2 only ever gains fields and routes; nothing is renamed or removed.
- **Auth.** `Authorization: Bearer <token>` with an API token created under
  Account → Tokens.
- **API switch.** The administrator's API switch (`api_enabled`) governs v1 and
  v2 together. While it is off, v2 answers `403` with `code: 20003` and
  `data.reason: "api_disabled"`. API tokens obey the switch on the console's
  `/api` routes too, and never reach `/api/admin/*`, `/api/tokens` or
  `PATCH /api/auth/password` (`403`, `data.reason: "api_token_forbidden"`).
- **No EXIF.** Like v1, v2 never returns EXIF or GPS data.

| Method | v2 path | Same as | Notes |
| --- | --- | --- | --- |
| GET | `/api/v2/profile` | `GET /api/auth/me` | Current account and usage |
| GET | `/api/v2/policies` | `GET /api/policies` | Upload rules this account may use |
| POST | `/api/v2/upload` | `POST /api/upload` | multipart `file`, optional `policy_id`, `album_id`, `is_public` |
| GET | `/api/v2/images` | `GET /api/images` | Paging, filters and unified search (`qv=1&q=...`) |
| GET | `/api/v2/images/{id}` | `GET /api/images/{id}` | |
| PATCH | `/api/v2/images/{id}` | `PATCH /api/images/{id}` | `is_public` and/or `name` (display name; links never change) |
| DELETE | `/api/v2/images/{id}` | `DELETE /api/images/{id}` | Moves to the recycle bin |
| POST | `/api/v2/images/batch` | `POST /api/images/batch` | `delete`, `permission`, `album` (move) |
| GET | `/api/v2/albums/{id}/images` | `GET /api/albums/{id}/images` | Search within one album |
| GET | `/api/v2/trash` | `GET /api/trash` | |
| POST | `/api/v2/trash/restore` | `POST /api/trash/restore` | |
| GET | `/api/v2/albums` | `GET /api/albums` | |
| POST | `/api/v2/albums` | `POST /api/albums` | |
| PATCH | `/api/v2/albums/{id}` | `PATCH /api/albums/{id}` | |
| DELETE | `/api/v2/albums/{id}` | `DELETE /api/albums/{id}` | Images stay, leaving the album |

Example:

```sh
curl -H "Authorization: Bearer $TOKEN" -F file=@photo.jpg https://img.example.com/api/v2/upload
```
