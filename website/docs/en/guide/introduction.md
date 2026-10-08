# What is ImgNest

ImgNest is a self-hosted image host with a Go backend and a Vue 3 interface. It aims to replace the community edition of Lsky Pro: your data stays on your own server and buckets, and your existing upload clients keep working.

## What it does

- **Multiple users, storages and policies.** Each user group can be bound to different storages and upload policies. Storage can be local disk or any S3-compatible service.
- **Original plus WebP.** The WebP copy and the thumbnail are produced during the upload request, so every link in the response is already reachable.
- **Lsky Pro v1 API compatibility.** PicGo, uPic, Typora and similar clients only need a new endpoint URL.
- **Privacy by default.** Full EXIF is kept in the local database only. Originals stored in the cloud have GPS, serial numbers and author fields erased without re-encoding.
- **Recoverable deletion.** Deleted images go to a recycle bin for 7 days by default, and the original URL stops working immediately.
- **Two layouts.** A classic layout and a new one, switchable at runtime, both with dark mode and Chinese/English.

## Current status

::: warning No stable release yet
ImgNest is under active development and version 1.0.0 has not been released. A Docker image is available: the `edge` tag follows the latest commit on `main` and may be unstable.
:::

What is implemented:

| Area | Contents |
| --- | --- |
| Accounts | Users, groups, tokens, login rate limiting, captcha |
| Storage | Local storage, S3-compatible storage, upload policies and path templates |
| Image processing | WebP conversion, two thumbnails, EXIF archiving and lossless scrubbing |
| Management | Recycle bin, albums, public gallery, admin panel |
| Compatibility | Lsky Pro v1 API |

Not done yet: a migration tool (`import-lsky`) that imports users, albums and image records from Lsky Pro is planned but not part of v1.

## Built with

| Part | Choice |
| --- | --- |
| Language | Go 1.27 |
| Image processing | libvips 8.18 through vipsgen |
| Database | SQLite or PostgreSQL |
| HTTP | Gin |
| Interface | Vue 3, Vite, TypeScript, Naive UI |
| Deployment shape | One Docker image with the interface built in; SQLite and PostgreSQL share the same image |

## Next

- To run it, read [Getting started](./getting-started).
- To connect PicGo, read [Lsky API and PicGo](./lsky-api).
- To learn where images are stored and how they are named, read [Storage and policies](./storage-and-policies).
