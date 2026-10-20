# User guide

Find the task you want to complete. Start with Getting started for a new installation, or Uploading and managing images if you already have an account.

::: tip Guide and versions
This guide follows `main`. A fixed release image does not gain later features automatically. If your interface differs, check Administration → Site settings → About first. The `edge` image follows `main`; choose an appropriate version for deployment.
:::

## First steps

1. [What is ImgNest](./introduction.md): capabilities, version and limits.
2. [Getting started](./getting-started.md): start the server, create an administrator and storage, upload your first image.
3. [Uploading and managing images](./upload-and-images.md): upload, copy links, find images and use bulk actions.

## Find your task

| I want to… | Where to open it | Instructions |
| --- | --- | --- |
| Upload images and copy links | Upload images `/upload` | [Uploading and managing images](./upload-and-images.md) |
| Organize albums and move images | My albums `/albums`; select images to move them | [Managing albums](./albums-and-gallery.md) |
| Search by filename, format or date | My images `/images` → search box | [Search syntax](./search.md) |
| Show a random album image at one URL | My albums → cover or name → Random image link | [Random image links](./random-links.md) |
| Inspect EXIF, delete or restore images | My images → image details / Recycle bin tab | [Privacy and recycle bin](./privacy-and-trash.md) |
| Connect PicGo, Typora or uPic | Account & tokens `/tokens` → Create Token | [Lsky API and PicGo](./lsky-api.md) |
| Write upload scripts or use responsive images | Call the API; an admin enables responsive variants in a policy | [Native API v2](./api-v2.md) |
| Configure quotas and upload permissions | Administration → User groups / Users | [Users and groups](./users-and-groups.md) |
| Set up storage, paths and conversion | Administration → Storage / Upload policies | [Storage and policies](./storage-and-policies.md) |
| Configure visitor access and guest uploads | Site settings → Guests & gallery | [Visitors and public access](./public-access.md) |
| Configure registration, CAPTCHA and mail | Site settings → Sign-up & sign-in / Email | [Sign-in protection](./sign-in-protection.md) |
| Deploy, upgrade or configure startup | Server terminal, Compose or configuration file | [Deployment](./deployment.md) |
| Import from Lsky | Site settings → Backup / export → Lsky migration at the bottom | [Migrating from Lsky](./lsky-migration.md) |
| Back up the database and configuration | Site settings → Backup / export → Back up now | [Backup and restore](./backup.md) |
| Change your password or inspect account activity | Account settings `/account/settings`; Overview `/dashboard` | [Account and usage](./account-and-overview.md) |
| Change layout, language or colors | New layout → sidebar gear → Settings → Appearance | [Interface and themes](./frontends.md) |

Expand collapsed navigation first; on a phone, open navigation. Paths such as `/albums` belong to your own ImgNest site, not this documentation site. Administration entries require an administrator. See [Responsive images API](./responsive-images.md) for blog integration and [Configuration](./configuration.md) for startup parameters.

## Where information belongs

- This guide is for users and administrators. Each topic owns its instructions; related topics link to it.
- [OpenAPI](https://github.com/biliblihuorong/imgnest/blob/main/docs/openapi.yaml) defines detailed API fields and error codes.
- The repository's [`docs/`](https://github.com/biliblihuorong/imgnest/tree/main/docs) is for developers. `planning/`, `superpowers/` and `verification/` contain development records, not a reading order for users.
