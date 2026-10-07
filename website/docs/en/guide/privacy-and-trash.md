# Privacy and recycle bin

Photos often carry the place they were taken and the serial number of the device. ImgNest keeps the full record in your own database and cleans the sensitive parts out of the files it puts in storage.

## How EXIF is handled

An upload is processed in this order:

1. All EXIF and XMP data is read and saved to the local database.
2. The original that goes to storage is cleaned according to the policy's `scrub_mode`.
3. The WebP copy and the thumbnail are generated. Neither carries EXIF or GPS; only the ICC colour profile is kept.

Cleaning is **lossless**. The metadata section of the file is rewritten in place, and pixels are neither decoded nor re-encoded, so image quality is identical to the file you uploaded.

## Scrubbing levels

| `scrub_mode` | Removed | Kept |
| --- | --- | --- |
| `gps` (default) | GPS, author and owner, body and lens serial numbers, the whole XMP block, vendor-private data | Camera model, lens, exposure settings, capture time, orientation, ICC |
| `all` | All EXIF | Orientation and ICC |
| `none` | Nothing | Everything |

::: warning HEIC and AVIF
In-place cleaning is not available for these two formats yet. A policy can store only the WebP copy (the default), store the file as is, or reject the upload.
:::

If cleaning fails, the upload fails. An uncleaned original is never stored.

## Who can see EXIF

| Where | Visible |
| --- | --- |
| The image owner and administrators | Full EXIF, GPS included |
| Public gallery | No |
| Lsky Pro v1 API | No |
| The original file in storage | Whatever remains after `scrub_mode` |

GPS coordinates are never written to logs.

## Recycle bin

Deleting an image does not erase it straight away:

1. **Delete.** The original, the WebP copy and the thumbnail move to a trash area in the storage, and the old addresses return 404 immediately. The space is deducted from your quota right away.
2. **Keep.** Images stay for 7 days by default. During that time they can be previewed and restored from the recycle bin.
3. **Restore.** The files move back and the original links work again. An image in the recycle bin still holds its path, so a new upload cannot take it.
4. **Expire.** An hourly job removes expired images: files, thumbnails and database records together.

You can also select images in the recycle bin and delete them permanently without waiting.

::: tip Change the retention period
Set the number of days under **Administration → Site settings**. Zero turns the recycle bin off, and deletion becomes immediate.
:::

### Why not just mark it in the database

Images on S3-style storage are served by the bucket's own domain and never pass through ImgNest. If deletion only set a flag in the database, the file would stay where it is and the link would keep working. Deletion therefore has to move the file.

### Backblaze B2

On B2, a delete without a version ID only adds a delete marker. Older versions remain and are still billed. When ImgNest deletes permanently, it lists every version of the file and deletes each one. A bucket lifecycle rule that keeps only the latest version is a sensible safety net.

CDN caches are outside ImgNest's control. Purge them on the CDN side if needed.

## When an upload fails

One upload writes several files. If any write fails, or the final database transaction fails, every file already written for that upload is removed. No orphan files are left without a record.

On restart, unfinished uploads, deletions and restores are completed before the server accepts requests.
