# Lsky Pro v1 — request / response samples

Values are illustrative; field names, nesting and types are the contract.
Fields marked *(ImgNest extra)* do not exist in Lsky and are safe additions.

## POST /api/v1/tokens

Request (form or JSON): `email=a@b.com&password=secret`

```json
{ "status": true, "message": "success", "data": { "token": "1|Xq8yXlkbN3k0pVd5fM9Rw2eT7uYhG6iC4oPzAa1s" } }
```

Wrong credentials (HTTP 200):

```json
{ "status": false, "message": "The email address or password is incorrect.", "data": {} }
```

## DELETE /api/v1/tokens

```json
{ "status": true, "message": "success", "data": {} }
```

## GET /api/v1/strategies

```json
{ "status": true, "message": "success", "data": { "strategies": [ { "id": 1, "name": "B2 默认" }, { "id": 2, "name": "本机" } ] } }
```

## POST /api/v1/upload

Multipart: `file` (required), `strategy_id`, `album_id`, `permission` (0/1).

```json
{
  "status": true,
  "message": "上传成功",
  "data": {
    "key": "GDmF2b",
    "name": "66ff1b2a3c4d5.png",
    "pathname": "2026/10/04/66ff1b2a3c4d5.png",
    "origin_name": "screenshot.png",
    "size": 245.5,
    "mimetype": "image/png",
    "extension": "png",
    "md5": "9e107d9d372bb6826bd81d3542a419d6",
    "sha1": "2fd4e1c67a2d28fced849ee1bb76e7391b93eb12",
    "links": {
      "url": "https://img.example.com/2026/10/04/66ff1b2a3c4d5.webp",
      "html": "&lt;img src=\"https://img.example.com/2026/10/04/66ff1b2a3c4d5.webp\" alt=\"screenshot.png\" title=\"screenshot.png\" /&gt;",
      "bbcode": "[img]https://img.example.com/2026/10/04/66ff1b2a3c4d5.webp[/img]",
      "markdown": "![screenshot.png](https://img.example.com/2026/10/04/66ff1b2a3c4d5.webp)",
      "markdown_with_link": "[![screenshot.png](https://img.example.com/2026/10/04/66ff1b2a3c4d5.webp)](https://img.example.com/2026/10/04/66ff1b2a3c4d5.webp)",
      "thumbnail_url": "https://img.example.com/2026/10/04/66ff1b2a3c4d5_thumbs.webp",
      "origin_url": "https://img.example.com/2026/10/04/66ff1b2a3c4d5.png",
      "webp_url": "https://img.example.com/2026/10/04/66ff1b2a3c4d5.webp"
    }
  }
}
```

`origin_url`, `webp_url` *(ImgNest extra)*. `size` is KB (float).
`md5`/`sha1` describe the stored original (after GPS scrubbing).

Failure examples (HTTP 200): quota exceeded, extension not allowed, file too
large — `{"status": false, "message": "<reason>", "data": {}}`.

## GET /api/v1/images?page=1&order=newest&permission=all&album_id=&keyword=

```json
{
  "status": true,
  "message": "success",
  "data": {
    "current_page": 1,
    "data": [
      {
        "album": null,
        "key": "GDmF2b",
        "name": "66ff1b2a3c4d5.png",
        "pathname": "2026/10/04/66ff1b2a3c4d5.png",
        "origin_name": "screenshot.png",
        "size": 245.5,
        "mimetype": "image/png",
        "extension": "png",
        "md5": "9e107d9d372bb6826bd81d3542a419d6",
        "sha1": "2fd4e1c67a2d28fced849ee1bb76e7391b93eb12",
        "width": 1920,
        "height": 1080,
        "links": { "url": "…", "html": "…", "bbcode": "…", "markdown": "…", "markdown_with_link": "…", "thumbnail_url": "…" },
        "human_date": "3 分钟前",
        "date": "2026-10-04 14:05:09"
      }
    ],
    "first_page_url": "https://img.example.com/api/v1/images?page=1",
    "from": 1,
    "last_page": 1,
    "last_page_url": "https://img.example.com/api/v1/images?page=1",
    "links": [],
    "next_page_url": null,
    "path": "https://img.example.com/api/v1/images",
    "per_page": 40,
    "prev_page_url": null,
    "to": 1,
    "total": 1
  }
}
```

`album` is `null` or `{"id": 3, "name": "博客", ...}`. `date` is
`Y-m-d H:i:s` in the server time zone; `human_date` is relative text.

## DELETE /api/v1/images/{key}

```json
{ "status": true, "message": "success", "data": {} }
```

## GET /api/v1/albums

Laravel paginator like images; each item: `{"id", "name", "intro", "image_num"}`.

## DELETE /api/v1/albums/{id}

```json
{ "status": true, "message": "删除成功", "data": {} }
```

## GET /api/v1/profile

```json
{
  "status": true,
  "message": "success",
  "data": {
    "name": "shell",
    "avatar": "https://img.example.com/avatar.png",
    "email": "a@b.com",
    "capacity": 10485760.0,
    "used_capacity": 2048.5,
    "url": "",
    "image_num": 120,
    "album_num": 4,
    "registered_ip": "203.0.113.5"
  }
}
```

`capacity` / `used_capacity` in KB. ImgNest maps `capacity` from the group's
`capacity_bytes / 1024` (0 = unlimited → return 0).

## Error statuses

| HTTP | When | Body |
| --- | --- | --- |
| 401 | missing/invalid token on a protected route, or bad token on upload | `{"status": false, "message": "Unauthenticated.", "data": {}}` |
| 403 | API disabled in settings | `{"status": false, "message": "管理员未启用 API", "data": {}}` |
| 429 | throttled (tokens: 3/min) | `{"status": false, "message": "Too Many Attempts.", "data": {}}` |
