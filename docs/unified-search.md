# Unified image search v1.1

The new Vben personal image page and album-detail page use one editable query.
The legacy `web/` application, trash and administrator global image list retain
their existing protocols and interfaces.

## Grammar and examples

```
暑假 海边 format:jpg,png album:"暑假照片" after:2026-10-01
"家庭 合照" camera:"Canon EOS" minsize:1MB maxsize:20MB
album:#42,#57 sort:oldest
after:2026-10-01 before:2026-11-01 visibility:private
```

Ordinary words are literal substrings of the displayed original filename.
Multiple words and different conditions combine with AND. Only format and album
comma lists combine their members with OR. There are no boolean expressions,
regular expressions, wildcard syntax, OCR, path or metadata searches hidden in
ordinary words. `%`, `_`, `*`, `AND`, `OR`, and `NOT` remain literal text.

| Field | Accepted values |
| --- | --- |
| format | jpg, png, gif, webp, avif, bmp, tiff, svg, heic, heif, unknown |
| album | Exact authorized name, `#` followed by a positive decimal ID, or unquoted `unfiled` |
| camera | One text value matched against normalized make/model or lens |
| minsize / maxsize | Inclusive decimal MB bounds, 0–1024000MB, at most six decimal places |
| after / before | Strict Gregorian YYYY-MM-DD, 1970-01-01 through 9999-12-31 |
| visibility | all, public, private |
| sort | newest, oldest, size-desc, size-asc, name-asc, name-desc |

- 1 MB is exactly 1,000,000 bytes. `0.000001MB` is one byte; zero is a real bound.
- Format describes trusted actual primary-object MIME, not the original filename extension. It does not expand upload permissions.
- Text uses NFC normalization and ASCII A–Z folding only. Album names resolve with exact NFC equality, without case folding.
- `after` includes local midnight; `before` excludes local midnight. Dates refer to upload created_at, not EXIF capture time. The required IANA timezone remains in the URL, and DST days are not assumed to have 24 hours.
- Quotes surround complete words or values; inside quotes only `\"` and `\\` escapes are accepted. Album names containing commas must be quoted.
- `album:#42` is an ID; `album:"#42"` is a name. `album:unfiled` is unassigned images; `album:"unfiled"` is a literal name.
- Repeated format/album fields merge. Repeated scalar fields are errors, even when identical. Unknown fields or partial invalid queries never silently disappear.

Limited Lsky-inspired input aliases normalize to this grammar:

| Alias | Canonical |
| --- | --- |
| extension:jpg,png | format:jpg,png |
| is:public / is:private | visibility:public / visibility:private |
| order:earliest | sort:oldest |
| order:utmost / order:least | sort:size-desc / sort:size-asc |
| order:created_at / order:created_at-asc | sort:newest / sort:oldest |

These are explicitly bounded aliases from different Lsky versions, not a general
Lsky compatibility language. `name:` is not a filename qualifier. `$旅行` is
ordinary text, not an album selector.

## Protocol, scope and limits

`GET /api/images?qv=1&q=...&tz=Asia%2FShanghai&page=1&size=20` is the new mode.
`q` must be present and may be empty. Page is 1–10000, size is 20, 50 or 100.
The old keyword/album_id/order/size/date filtering protocol cannot be mixed with
qv=1. Without qv the legacy behavior is preserved, including the old q needle.

Album details use `GET /api/albums/{id}/images` with the same query parameters.
The route album is independently owner-verified and remains a fixed boundary
when the query is cleared or changed. A query cannot widen this scope.

Successful new-mode responses add `data.search` with appliedVersion=1,
canonicalQ, tz, authorizedAlbums and appliedRange. Canonical album references
use decimal string IDs, so renaming an album does not change an existing link.
The frontend rejects a missing/mismatched appliedVersion instead of trusting an
unfiltered response. Authenticated personal lists stay personal even for admins.

`GET /api/albums/suggestions` accepts keyword, page, size≤20 and an optional
scope_album_id for album details. It returns only string IDs, names and hasMore.
Suggestions do not replace exact database resolution when executing a query.
Missing and inaccessible albums return the same ALBUM_NOT_AVAILABLE diagnostic.

The raw and canonical queries must both fit 4096 UTF-8 bytes so mandatory
quoting and resolved album IDs cannot create an unusable link; canonical
expansion over the cap returns QUERY_TOO_COMPLEX before executing SQL. The raw
query is also capped at 32 tokens. There are at most 16
raw ordinary terms and 8 distinct ones, 200 Unicode code points per text value,
10 raw album entries and 5 resolved distinct entries. Error source spans use
original JavaScript UTF-16 code units. Up to five diagnostics are returned in
`data.diagnostics` with stable code/messageKey/span/args; outer native code stays
numeric. Syntax/parameter errors are 400; unavailable/ambiguous albums are 422.

## Interaction contract

Typing parses locally. Submit applies valid conditions; completion and example
insertion modify the draft without submitting. Chips edit that same text.
Refresh reruns the applied query, and failures preserve the last successful
result with a visible stale-result indication. Browser history restores the
committed query and pagination. IME confirmation and selecting a completion do
not accidentally submit. Cancelling a request never displays an error toast.

The ? button exposes syntax, examples, aliases and timezone without leaving the
page. It is keyboard accessible, restores focus on close, and uses a scrollable
mobile presentation. Existing selection, copy menus, previews, batch actions,
page-size controls and refresh stay available.

## Storage and controlled migration

Migration 0005 adds nullable normalized search columns and focused owner/scope
indexes. Existing trusted MIME/size/created_at fields are reused; no inferred
actual-format column is populated from filename suffixes.

The explicit `migrate` command performs a bounded 250-row NFC/ASCII backfill.
NULL marks unfinished rows, each completed batch commits, and rerunning resumes
unfinished work. Original names and EXIF are preserved. Normal writes maintain
the derived columns; request handling never starts a bulk backfill. Startup
rejects an incomplete migration/backfill instead of serving wrong search data.

Before upgrading, stop writes, back up the database and matching storage/config,
then test the new binary and migration on a copy. Never alter published migration
files or use AutoMigrate. For rollback, stop the new binary and restore the
pre-upgrade database snapshot with the previous binary; the old migrator rejects
unknown 0005/0006 versions, so merely swapping executables is not a safe rollback.
Images uploaded after the backup require an explicit reconciliation plan before
restoring that snapshot. No production migration is part of the implementation
test run.

## Verification

Go and TypeScript consume the same parser vectors in
`internal/searchquery/testdata/query-v1.json`. Regression coverage must include
full-dataset filtering before pagination, matching count, cross-user/album
authorization, name ambiguity, literal SQL wildcard characters, DST and skipped
civil dates, exact byte bounds, canonical round trips, URL history, request races,
input methods, help focus, and existing image operations.

Run the repository's fixed toolchain tests and check both SQLite and PostgreSQL.
Performance acceptance uses isolated 10k/100k datasets and records query/count
timings and plans; the suggested 100k typical-filter p95 target is 500ms, not an
assumed result. Browser screenshots and real rendering/accessibility checks must
be reported separately from jsdom or HTTP-only tests. See the [verification report](verification/2026-10-05-search-accounts.md)
for actual results and environment limitations.
