# Migrating from Lsky Pro

Upload the `.sql` export of your Lsky Pro database under **Site admin → Settings → Backup / export → Lsky migration** to import its groups, users, tokens, albums and images. Every image **runs through the regular ImgNest upload pipeline under its original path**: format detection, EXIF archiving, WebP, local and cloud thumbnails, responsive versions (when the rule enables them) and quota, exactly like a new upload.

Images can be imported in two ways:

| | Adopt in place (recommended) | Download and store again |
| --- | --- | --- |
| Originals | Stay in the S3 bucket Lsky used, **byte for byte** | Downloaded from the old links into the rule's storage (usually a new bucket) |
| Old links | Keep working, nothing to switch | Switch the old domain to the new bucket afterwards |
| Files added | WebP, thumbnails and responsive copies next to each original | Original, WebP, thumbnails and responsive copies |
| GPS and similar EXIF in originals | Stays in the bucket files (ImgNest's public APIs never return it) | Removed losslessly per the rule, so `md5` differs from the Lsky record |
| Requires | The rule's storage is the old bucket, with versioning on (B2 always has it) | The ImgNest host can reach the old links |

## What is imported

| Lsky Pro | ImgNest | Notes |
| --- | --- | --- |
| `groups` | `groups` | A group with the same name is reused unchanged; new groups keep `maximum_file_size`, `limit_per_minute` and the accepted formats ImgNest supports, and default to the rule you import into |
| `users` | `users` | An account with the same email is reused (password, role and group untouched); new accounts keep the bcrypt password (`$2y$` verifies as is), admin flag, registration IP and creation time. Names shorter than 3 characters fall back to the email's local part, with the original kept as display name |
| `personal_access_tokens` | `tokens` | The digest is kept, and so is the id when it is free, so `id\|secret` tokens in PicGo or Typora keep working |
| `albums` | `albums` | An album with the same owner and name is reused |
| `images` | `images` + objects | Path (`pathname`) and extension are kept, as are `created_at` (broken year-1 values are recovered from the `uniqid` file name), visibility, upload IP and original file name |
| `configs`, strategy keys | not imported | Only each strategy's public `url` is kept; storage keys and the SMTP password are dropped while parsing, never stored or shown |

Not imported: guest uploads, images of disabled users, formats ImgNest cannot store (`.ico`, `.psd`), and images whose extension would change on save (`.jpeg`, upper-case extensions, and, when copying, content not matching the extension), because their old links would break. PNG/JPEG files Lsky only renamed to `.webp` without converting are imported: adopted as they are (same link, no extra WebP), or stored as a real WebP when copying. The import checks each file against its owner group's per-file limit, which can be as large as you like (raise it under User groups), not `server.max_upload_mb`; a group without its own limit allows up to 100 MiB. Files over 20 MiB get no EXIF record. When the rule scrubs EXIF they can only be adopted, because copying must scrub their EXIF, which works up to 20 MiB; with scrubbing off they can be copied too. The page lists each of them at the end; they stay in the old storage (and when adopting, their old links keep working).

::: tip Matching your administrator account
Accounts match by email. Create the ImgNest administrator with the same email as your Lsky administrator and the imported images and tokens land on that account, which keeps its ImgNest password. With a different email, a second administrator account with the Lsky password is created.
:::

## Importing from the console

1. **Export the Lsky database**: dump the whole PostgreSQL database to a `.sql` file with `pg_dump` (plain SQL) or DBX or Navicat. The export holds password hashes and storage keys; delete it when you are done and never share it.
2. **Prepare the storage and rule**:
   - Adopting: add the S3 bucket Lsky used under Storages (for B2 the endpoint is `https://s3.<region>.backblazeb2.com` with Path Style on) and **set its Base URL to the URL in the Lsky strategy** (for example `https://img.example.com/ShellBlogImg`) so old links and ImgNest links match. If the files sit under a folder of the bucket (keys like `ShellBlogImg/1/2025/...`), set the path prefix to that folder. If unsure, leave it empty: the import first checks a few images and tells you what to set when it cannot find them.
   - Storing again: add a new bucket. Its rule's WebP mode must not be WebP only, or original links would be lost.
   - Then create a rule on that storage with thumbnails and responsive versions on.
3. **Upload the export**: open **Site admin → Settings → Backup / export → Lsky migration**, check the time zone (default `Asia/Shanghai`) and choose the `.sql` file. It is parsed in server memory and never written to disk; the page shows how many users, images and storage strategies it holds.
4. **Choose the mode and rule**: when adopting, pick only the Lsky storage strategy kept in this bucket (it is preselected when the Base URLs match); images of other strategies would fail because the bucket does not have them.
5. **Start the import**: progress refreshes every two seconds and you can stop at any time. At the end the page lists failed and skipped images; fix the cause and press Start import again, and only what is still missing is retried. After a server restart, upload the export again.

::: warning After adopting
ImgNest now manages these images: deleting one in ImgNest makes its old link return 404 at once (the original stays in the bucket as an older version and a restore writes it back), and when the recycle bin expires or the image is purged the original is deleted too. Do not keep Lsky writing to the same bucket.
:::

Your reverse proxy (for example 1Panel's OpenResty) must accept uploads larger than the export (`client_max_body_size` in Nginx); ImgNest itself accepts exports up to 128 MiB.

## Importing from the command line

`imgnest import-lsky` does the same without the console:

```bash
# Read the export file directly (or load it into PostgreSQL and point IMGNEST_LSKY_DSN at it)
docker compose run --rm -v "$PWD/LsyPro.sql:/tmp/LsyPro.sql:ro" imgnest import-lsky --dump /tmp/LsyPro.sql --dry-run
# Adopt in place: the rule's storage is the old bucket
docker compose run --rm -v "$PWD/LsyPro.sql:/tmp/LsyPro.sql:ro" imgnest import-lsky --dump /tmp/LsyPro.sql --policy-id 1 --adopt --limit 5
# After a spot check of those 5, drop --limit to import the rest
```

To store again instead, drop `--adopt`. Originals are downloaded from each strategy's `url`; `--source-url` overrides it, or sync the old bucket to a local directory and use `--source-dir`. Stop the server during a command-line import (`docker compose stop imgnest`) and start it afterwards.

### Storing again: switching the old links

1. During the import your site keeps reading the old bucket.
2. After a spot check, point the old link origin (CDN or reverse proxy, e.g. `img.example.com/old-bucket`) at the new bucket and set the storage's Base URL in ImgNest to the old domain. Old links are now served by ImgNest-managed objects.
3. Copy skipped files such as `.ico` from the old bucket to the new one (rclone or similar) before switching, so their links keep working.

## Afterwards

- Accounts whose Lsky password is not bcrypt are imported unable to sign in; reset them with `imgnest reset-password --email ...`.
- When a token id is taken, the token gets a new id and the report tells you to change `old-id|` to `new-id|` in the client.
- Only PostgreSQL Lsky databases are supported; convert a MySQL one first (for example with pgloader).
- After the migration, replace the old Backblaze application key and the SMTP password the Lsky configuration used.
