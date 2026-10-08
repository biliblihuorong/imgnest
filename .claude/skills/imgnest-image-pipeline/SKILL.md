---
name: imgnest-image-pipeline
description: ImgNest upload and image-processing rules — path templates ({Y}/{m}/{uniqid}…), the original + WebP + thumbnail objects, libvips/vipsgen usage, EXIF extraction and lossless GPS scrubbing, recycle bin and B2 purge. Use whenever writing or reviewing code in internal/pathtpl, internal/imaging, internal/exif, internal/storage or the upload/delete services, or when changing how files are named, converted, stored or deleted.
---

# ImgNest image pipeline

Source of truth: `docs/spec.md` sections 5 and 6. This skill is the working
checklist; if the two disagree, the spec wins and this file must be updated.

## The upload flow (order matters)

1. **Authorise and check limits** — group `max_file_bytes`, `allowed_exts`,
   remaining capacity, per-minute rate. Fail before reading the whole body.
2. **Read into memory** with a hard cap (`server.max_upload_mb`).
3. **Detect the real format** from magic bytes + libvips probe. Never trust the
   file name or `Content-Type`. Reject SVG. Enforce the pixel limit
   (default 100 MP, frames included).
4. **Hash** the uploaded bytes → `src_md5` (dedupe key).
5. **Read EXIF** (`internal/exif`) → `image_exif` row, GPS included.
6. **Render the path** with `pathtpl.Build(policy.PathTpl, policy.NameTpl, vars)`.
   On a `(storage_id, path)` collision: re-render if the template has a random
   variable (max 5 tries), otherwise follow `on_conflict` (`rename` → `-1`,
   `-2`…, or `reject`).
7. **Prepare the cloud original** — apply `scrub_mode` (see below). Hash the
   bytes that will actually be stored → `md5` / `sha1`.
8. **Make WebP** per `webp_mode`; skip storing it if larger than the original
   and `skip_if_larger` is on (except `webp_only`).
9. **Make the thumbnail** (static WebP, long edge `thumb_size`, default 400).
10. **Write objects**, then the DB row in one transaction (image + exif +
   `users.used_bytes += unique_cloud_object_bytes`). Original/WebP/cloud
   thumbnail all count; an uploaded WebP's shared key counts once. Local
   thumbnail cache does not count. Short reservation/commit transactions
   surround storage IO; pending image rows hold path and quota reservations.
11. **Compensate**: if any write or the transaction fails, delete every object
    already written in this request. Never leave orphans.

Persist each object's SHA256 with its write intent. Foreground compensation,
background cleanup, recycle-bin IO and preview-cache rebuilding share the
single-instance lifecycle fence. Re-read state/operation after acquiring it;
finish all physical IO before releasing a path. Old cache writes and second
cleaners must never affect a replacement image at the same path.

## Object keys (path has no extension)

| File | Key |
| --- | --- |
| Original | `{path}.{ext}` (`jpeg` → `jpg`, lower case) |
| WebP | `{path}.webp` |
| Cloud thumbnail | `{path}_thumbs.webp` |
| Local thumbnail | `data/thumbs/{storage_id}/{path}_thumbs.webp` (served at `/t/{key}.webp`) |
| Recycle bin | `_trash/` + any of the above |

`_thumbs` is a reserved name suffix; random filenames rerender up to 5 times,
deterministic names append `-1`. Internal `_trash`/`.trash` namespaces cannot
be rendered by user templates. URLs are never stored: base URL plus correctly
escaped key segments. Private images hide public display; their direct links
remain accessible, as explicitly chosen by the user.

## Path template variables

Lsky-compatible: `{Y} {y} {m} {d} {timestamp} {uniqid} {md5} {md5-16}
{str-random-16} {str-random-10} {filename} {uid}`.
ImgNest additions: `{H} {i} {s} {sha1} {uuid} {rand:N}` (1–64) and
`{hash:N}` (1–32, content-MD5 prefix, for spreading files across folders).
`{md5}` is the **content** hash here (Lsky used a random value).
Validate templates when a policy is saved; unknown variables are errors.

Sanitising: drop `.`/`..`/empty segments, strip control chars, replace
whitespace and `?#%&\:*"<>|` with `-`, keep CJK, ≤100 runes per segment,
≤255 bytes total.

## libvips via vipsgen

- Import exactly one package and match it to the libvips in the image:
  `github.com/cshum/vipsgen/vips` = libvips 8.18.x (`vips817`, `vips816` for
  older). Pin both in the Dockerfile.
- `vips.Startup` once; set `MaxCacheFiles/Mem/Size = 0` (no op cache) and
  `ConcurrencyLevel: 1`; bound parallelism with a semaphore sized to CPUs.
- Detect magic before loading; use N=-1 only for animation loaders. Do not
  silently fall back to first-frame loading after arbitrary errors. Loaded
  frames = Height()/PageHeight(); Pages is document count, not loaded frames.
  Pixel budget = width * frame height * loaded frames (no double counting).
- Convert with `NewThumbnailBuffer(buf, w, &ThumbnailBufferOptions{Height: h,
  Size: SizeDown})`: it auto-rotates from EXIF orientation and only shrinks.
  Use `w = h = 10_000_000` for "no resize". For animated GIF/WebP set
  `OptionString: "n=-1"` (only for those loaders — JPEG rejects `n`).
- Save with `WebpsaveBuffer` — defaults Q 80, effort 4, `Keep: KeepIcc` for
  WebP (drops EXIF/XMP/GPS), `Keep: KeepNone` for thumbnails.
- Always `defer img.Close()`.
- The pinned imagor-base disables Magick. Rebuild the same vips8.18.6 official
  tarball with its fixed digest and Magick enabled for BMP; never upgrade
  vips/vipsgen to hide a missing-loader failure.
- Runtime env in the container: jemalloc via `LD_PRELOAD`,
  `MALLOC_ARENA_MAX=2`.

## EXIF: keep locally, scrub in the cloud

`image_exif` keeps everything (GPS, full EXIF + XMP as JSON in `raw`). GPS and
`raw` are visible only to the owner and admins; public, gallery and Lsky v1
responses never include EXIF.

`scrub_mode` for the **stored original**: `none` | `gps` (default) | `all`.
Scrubbing must be **lossless** — never re-encode pixels. Details and byte
layouts: [references/exif-scrub.md](references/exif-scrub.md).

Archive original EXIF/XMP/unknown/MakerNote blocks from container bytes;
imagemeta's structured result alone does not preserve all data. Avoid locked
vipsgen GetBlob until ownership is validated; its borrowed-pointer free risk
was found from source, not yet reproduced natively. GPS/all scrub removes
MakerNote identity payloads too. Reload and reparse scrubbed bytes; any failure
rejects the upload. Never keep an unmodified original under an enabled scrub
mode. WebP/thumbnails explicitly strip metadata; Keep(0) does not strip.

Reject missing container terminators even if native decoders repair them.
Opaque classic TIFF layouts may archive at most20MiB as explicit private
full-source-fallback; enabled original scrub rejects unsupported layouts.
BigTIFF and unsupported ISOBMFF layouts reject rather than lose metadata.
M2 fixtures are synthetic/native-generated, not camera-brand samples.

HEIC/AVIF: no in-place scrub in v1. `heif_mode`: `webp_only` (default) |
`keep` | `reject`.

## Thumbnails

Written twice. Admin list, preview and recycle bin read the **local** copy; API
`thumbnail_url` and the gallery use the **cloud** copy. Missing local thumbs
are regenerated lazily from the cloud thumbnail or WebP. The local thumbs
folder is a cache — safe to delete.

## Delete, restore, purge (recycle bin, default 7 days)

- **Delete**: set `deleted_at`, `purge_at = now + trash.days`; server-side
  copy to `_trash/<key>` then remove the original (local `.trash/<key>`).
  Validate byte identity, not just owner+size. Source-already-gone recovery
  stream-checks the target against persisted SHA256. Success requires the old
  URL404. Subtract `used_bytes` once.
- **Restore**: copy back, clear `deleted_at`. The row keeps holding its
  `(storage_id, path)` unique slot while in the bin, so restore never collides.
- **Restore** stays busy as restore_cleanup until trash copies are purged.
  Startup cancels unfinished old restores without forging user authorization.
- **Purge**: ImagePurger.PurgeImage first validates all entity-version owners,
  then removes original/trash versions and markers by VersionId and verifies
  the listing is empty. Foreign history keeps the operation pending. Only
  then remove local cache, EXIF and image row. Compensation instead uses
  OwnedPurger.PurgeOwned, preserving foreign history and clearing local owned
  staging even when canonical Stat is missing.
- CDN cache purging is out of scope: "old URL 404" means the origin, not
  browser/CDN copies of the immutable direct link.
- Trash copies are written with `Cache-Control: private, no-store`; restored
  objects go back to the long-lived public header. Public buckets must deny
  anonymous reads under `_trash/` (see `docs/deployment.md`).

## S3 client settings (COS / R2 / B2)

`RequestChecksumCalculation` and `ResponseChecksumValidation` =
`WhenRequired`; B2 and some COS/R2 setups reject the SDK's default CRC
headers. `UsePathStyle` per storage. Put objects with
`Cache-Control: public, max-age=31536000, immutable` (keys never change).

## Tests every change here should keep green

Conditional PUT and multipart completion are probed on the real backend. The
pinned MinIO ignores destination conditions on ordinary CopyObject, so copy
uses CreateMultipartUpload/UploadPartCopy/CompleteMultipartUpload(IfNoneMatch).
Unsupported vendor capability fails; MinIO success does not prove B2/COS/R2.

Local durable objects use a single owned envelope (IMGNST01, uint32 header
length, ObjectInfo JSON, unchanged image bytes); only Driver.Open serves their
payload. Never expose the physical root via a static file server. Preview cache
is still raw WebP. See internal/storage/README.md and docs/spec.md§8.2.

- `pathtpl`: table tests per variable, sanitiser cases, fuzz `Sanitize`
  (never yields `..` or a leading `/`).
- `imaging`: fixtures for JPEG with orientation 6, PNG with alpha, animated
  GIF (frames preserved), CMYK JPEG, 1×1 image, truncated file, >100 MP header.
- `exif`: phone JPEG with GPS → GPS parsed; scrubbed output has no GPS, same
  length, same pixels (decode both and compare), still loads in libvips.
- `storage`: local driver with `os.Root` cannot escape via `../` or symlinks;
  S3 driver against MinIO (put/copy/delete/purge with versioning on).
- Upload service: inject a failing storage on the 2nd write and assert the 1st
  object was removed (compensation).
