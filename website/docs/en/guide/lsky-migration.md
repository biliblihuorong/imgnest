# Migrating from Lsky Pro

`imgnest import-lsky` reads a Lsky Pro PostgreSQL database, maps its groups, users, tokens and albums into ImgNest's own tables, then downloads every image from its old link and **runs it through the regular ImgNest upload pipeline under its original path**: format detection, EXIF archiving, lossless scrubbing of the original, WebP, local and cloud thumbnails, responsive versions (when the rule enables them) and quota, exactly like a new upload. The result is plain ImgNest data, with no half-adopted legacy rows.

## What is imported

| Lsky Pro | ImgNest | Notes |
| --- | --- | --- |
| `groups` | `groups` | A group with the same name is reused unchanged; new groups keep `maximum_file_size`, `limit_per_minute` and the accepted formats ImgNest supports, and default to `--policy-id` |
| `users` | `users` | An account with the same email is reused (password, role and group untouched); new accounts keep the bcrypt password (`$2y$` verifies as is), admin flag, registration IP and creation time. Names shorter than 3 characters fall back to the email's local part, with the original kept as display name |
| `personal_access_tokens` | `tokens` | The digest is kept, and so is the id when it is free, so `id\|secret` tokens in PicGo or Typora keep working |
| `albums` | `albums` | An album with the same owner and name is reused |
| `images` | `images` + objects | Path (`pathname`) and extension are kept, as are `created_at` (broken year-1 values are recovered from the `uniqid` file name), visibility, upload IP and original file name |
| `configs`, strategy keys | not imported | Only each strategy's public `url` is read, to download originals; storage keys and the SMTP password are never read or printed |

Not imported: guest uploads, images of disabled users, formats ImgNest cannot store (`.ico`, `.psd`), and images whose extension would change on save (content not matching the extension, `.jpeg`, upper-case extensions), because their old links would break. The command lists each of them at the end; they stay in the old storage only.

::: tip Matching your administrator account
Accounts match by email. Create the ImgNest administrator with the same email as your Lsky administrator and the imported images and tokens land on that account, which keeps its ImgNest password. With a different email, a second administrator account with the Lsky password is created.
:::

## Why a new bucket

ImgNest only creates objects, with ownership metadata, and never overwrites them. The old Lsky objects already occupy the same keys, so images are written to a **new bucket** with the same paths and the old bucket is left untouched as a fallback:

1. During the import your site keeps reading the old bucket.
2. After a spot check, point the old link origin (CDN or reverse proxy, e.g. `img.example.com/old-bucket`) at the new bucket and set the storage's `base_url` in ImgNest to the old domain. Old links are now served by ImgNest-managed objects.
3. Copy skipped files such as `.ico` from the old bucket to the new one (rclone or similar) before switching, so their links keep working.

With the rule's default `scrub_mode` of `gps`, re-ingested originals lose GPS and other sensitive EXIF (losslessly, without re-encoding), so their `md5` differs from the Lsky record. That is expected.

## Steps (Docker Compose with PostgreSQL)

1. **Load the Lsky export into a temporary database** (it contains plain-text secrets, drop it afterwards):

   ```bash
   docker compose -f deploy/compose.postgres.yaml exec -T postgres createdb -U imgnest lsky
   docker compose -f deploy/compose.postgres.yaml exec -T postgres psql -q -U imgnest -d lsky < LsyPro.sql
   ```

2. **Prepare the target storage and rule in the admin panel**: add the new bucket as an S3 storage (needs `IMGNEST_SECURITY_MASTER_KEY`), then create a rule for it with WebP mode "original + WebP" (not WebP only, which drops the originals), thumbnails and responsive versions enabled. Note its ID.

3. **Stop the server and import**:

   ```bash
   docker compose -f deploy/compose.postgres.yaml stop imgnest
   export IMGNEST_LSKY_DSN="host=postgres user=imgnest password=$POSTGRES_PASSWORD dbname=lsky sslmode=disable"
   docker compose -f deploy/compose.postgres.yaml run --rm -e IMGNEST_LSKY_DSN imgnest import-lsky --dry-run
   docker compose -f deploy/compose.postgres.yaml run --rm -e IMGNEST_LSKY_DSN imgnest import-lsky --policy-id 2 --limit 5
   # check those five, then run everything
   docker compose -f deploy/compose.postgres.yaml run --rm -e IMGNEST_LSKY_DSN imgnest import-lsky --policy-id 2
   docker compose -f deploy/compose.postgres.yaml start imgnest
   ```

   Originals are downloaded from each strategy's `url` by default, so the machine must reach it. Alternatively sync the old bucket to a directory and pass `--source-dir` (mounted into the container). Lsky timestamps are read in `--timezone` (default `Asia/Shanghai`).

4. **Read the report**: one `OK` / `FAIL` line per image, then the skipped and failed images. Fix the cause and **rerun the same command**: imported images are skipped and only the rest is retried.

5. **Switch the old links** (see above), check your site, then drop the temporary database:

   ```bash
   docker compose -f deploy/compose.postgres.yaml exec -T postgres dropdb -U imgnest lsky
   ```

## Afterwards

- Accounts whose Lsky password is not bcrypt are imported unable to sign in; reset them with `imgnest reset-password --email ...`.
- When a token id is taken, the token gets a new id and the report tells you to change `old-id|` to `new-id|` in the client.
- Only PostgreSQL Lsky databases are supported; convert a MySQL one first (for example with pgloader).
