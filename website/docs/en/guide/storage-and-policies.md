# Storage and policies

A **storage** decides where images are kept. A **policy** decides how they are named, whether a WebP copy is made and how private metadata is handled. Each policy points at one storage, and users upload under the policies bound to their group.

## Storage

### Local storage

`init-local` creates a local storage together with a default policy:

```bash
docker compose -f deploy/compose.sqlite.yaml exec imgnest \
  imgnest init-local --base-url https://img.example.com
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `--base-url` | `http://localhost:8080` | Public origin; `/i/<storage id>` is appended |
| `--root` | `data/images` | Directory that holds the files; inside the `/app/data` volume in the image |
| `--name` | `local` | Storage name |

::: warning Do not publish the storage directory as static files
The `.jpg` and `.webp` files in a local storage directory use ImgNest's internal container format. They are not plain images and must be served through the program. Back up the whole directory.
:::

### S3-compatible storage

[Set a master key](./configuration#other-settings) first, then put the storage settings in a file you do not commit:

```json
{
  "name": "cloud",
  "driver": "s3",
  "base_url": "https://images.example.com",
  "config": {
    "endpoint": "https://s3.example.com",
    "region": "us-east-1",
    "bucket": "your-bucket",
    "access_key_id": "YOUR_ACCESS_KEY",
    "secret_access_key": "YOUR_SECRET",
    "use_path_style": true
  }
}
```

Create the storage from standard input, then add a policy for it:

```bash
docker compose -f deploy/compose.sqlite.yaml exec -T imgnest \
  imgnest init-storage < private-storage.json

docker compose -f deploy/compose.sqlite.yaml exec imgnest \
  imgnest init-policy --storage-id 2 --name cloud
```

Use the ID printed by the first command for `--storage-id`. Creation runs a real connection test: one write, one copy and one cleanup. Services that lack a required capability are rejected. Credentials are encrypted in the database and no API returns them afterwards.

You can also create and test storages under **Administration → Storage**.

## The files behind one image

A policy first produces a path without an extension, and several files derive from it:

| File | Location | Example address |
| --- | --- | --- |
| Original | `{path}.{ext}` in the storage | `https://img.example.com/26/10/66ff1b2a3c4d5.png` |
| WebP | `{path}.webp` in the storage | `https://img.example.com/26/10/66ff1b2a3c4d5.webp` |
| Cloud thumbnail | `{path}_thumbs.webp` in the storage | `https://img.example.com/26/10/66ff1b2a3c4d5_thumbs.webp` |
| Local thumbnail | `data/thumbs/` on the server | `/t/{key}.webp`, used only by admin lists and previews |

Addresses are not stored in the database. They are built from the storage's `base_url` on every request, so changing the domain only means editing the storage.

`_thumbs` is a reserved suffix. A file name cannot end with it.

## Path templates

A policy has two templates: one for the directory and one for the file name. The defaults follow Lsky Pro: <code v-pre>{Y}/{m}/{d}</code> for the directory and <code v-pre>{uniqid}</code> for the file name.

<div v-pre>

| Variable | Meaning | Example |
| --- | --- | --- |
| `{Y}` / `{y}` | 4-digit / 2-digit year | 2026 / 26 |
| `{m}` / `{d}` | Month / day, zero-padded | 10 / 04 |
| `{H}` `{i}` `{s}` | Hour / minute / second | 14 / 05 / 09 |
| `{timestamp}` | Unix seconds | 1791091509 |
| `{uniqid}` | 13-character time-ordered ID | 66ff1b2a3c4d5 |
| `{md5}` / `{md5-16}` | MD5 of the file content | 9e107d9d… |
| `{sha1}` | SHA-1 of the file content | 2fd4e1c6… |
| `{str-random-16}` / `{str-random-10}` | Random letters and digits | aZ3kP0qL9xW2bN7c |
| `{rand:N}` | N random lowercase letters and digits, N from 1 to 64 | `{rand:6}` gives k3x9a0 |
| `{hash:N}` | First N characters of the content MD5, for spreading files across directories | `{hash:2}` gives 9e |
| `{uuid}` | UUID v4 | 1b4e28ba-… |
| `{filename}` | Original file name, extension removed and sanitised | my-photo |
| `{uid}` | Uploader ID, 0 for guests | 3 |

</div>

::: tip One difference from Lsky Pro
In Lsky Pro <code v-pre>{md5}</code> is a random value. ImgNest uses the hash of the file content, which makes it usable for deduplication.
:::

Common combinations:

<div v-pre>

| Use | Directory template | File name template |
| --- | --- | --- |
| Personal blog | `{y}/{m}` | `{uniqid}` |
| Many images, avoiding huge directories | `{Y}/{m}/{hash:2}` | `{md5-16}` |
| Keep the original file name | `{Y}/{m}/{d}` | `{filename}` |

</div>

### Rules applied when building a path

- The extension comes from the real image format, not the uploaded file name. It is lower case, and `jpeg` becomes `jpg`.
- Whitespace and `? # % & \ : * " < > |` are replaced with `-`. Chinese characters are kept.
- On a name collision, templates with a random variable are regenerated up to 5 times. Templates without one get `-1`, `-2` appended to the file name.
- Templates are validated when the policy is saved. An unknown variable is an error.

## WebP and thumbnails

| `webp_mode` | What is stored | Good for |
| --- | --- | --- |
| `both` (default) | Original and WebP | Keeping originals while serving lighter links |
| `webp_only` | WebP only | Saving space on blog images |
| `none` | Original only | Photography originals and archival copies |

| Policy field | Default | Meaning |
| --- | --- | --- |
| `webp_quality` | 80 | WebP quality, 1 to 100 |
| `webp_lossless` | false | Lossless WebP, suited to screenshots and icons |
| `max_width` / `max_height` | 0 (no limit) | Shrinks the WebP copy only; the original is untouched |
| `thumb_enabled` / `thumb_size` | true / 400 | Longest edge of the thumbnail in pixels |
| `link_prefer` | `webp` | Which version the API returns as the main link |
| `scrub_mode` | `gps` | Privacy scrubbing level, see [Privacy and recycle bin](./privacy-and-trash) |

How each format is handled:

- JPEG, PNG, BMP, TIFF, HEIC and AVIF are converted according to the policy, after auto-rotating by EXIF orientation.
- Every frame of a GIF is converted into an animated WebP.
- An upload that is already WebP is not converted again.
- SVG uploads are rejected by default because SVG can embed scripts.

The format is decided from the file header and libvips probing only. The extension and `Content-Type` are not trusted.
