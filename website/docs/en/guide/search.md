# Search syntax

The personal images page and album detail page share one editable query language. Plain words are literal substrings of the displayed filename; multiple words and conditions combine with AND. Only `format:` and `album:` comma lists are OR inside the list. There are no boolean expressions, wildcards or regular expressions — `%`, `_`, `*`, `AND`, `OR` and `NOT` are ordinary text.

## Where to enter a query

1. Sign in and open My images (`/images`), or open an album from My albums.
2. Enter a query such as `beach format:jpg,png` in the search box above the image list, then click Search or press Enter.
3. Check the result count and applied conditions. Correct any reported syntax error and search again; clear the query to return to all images in the current scope.

Album searches stay within that album. The public gallery does not use this personal-library search entry.

## Examples

```
summer beach format:jpg,png album:"Summer 2026" after:2026-10-01
"family portrait" camera:"Canon EOS" minsize:1MB maxsize:20MB
album:#42,#57 sort:oldest
visibility:private album:unfiled before:2026-11-01
```

## Fields

| Field | Accepted values |
| --- | --- |
| `format` | jpg, png, gif, webp, avif, bmp, tiff, svg, heic, heif, unknown (comma list) |
| `album` | Exact album name, `#` + decimal ID, or unquoted `unfiled` |
| `camera` | One text value matched against normalized make/model or lens |
| `minsize` / `maxsize` | Inclusive MB bounds, 0–1024000, up to six decimals |
| `after` / `before` | `YYYY-MM-DD`; `after` includes local midnight, `before` excludes it |
| `visibility` | all, public, private |
| `sort` | newest, oldest, size-desc, size-asc, name-asc, name-desc |

Details:

- `1MB` is exactly 1,000,000 bytes; `0.000001MB` is one byte, and `0` is a real bound.
- `format` describes the probed **actual** format at upload time — not the filename extension — and never widens upload permissions.
- Dates refer to upload time (`created_at`), not EXIF capture time. The browser timezone travels with the link; DST days are not assumed to have 24 hours.
- Quotes wrap values with spaces or commas; only `\"` and `\\` escapes are accepted inside. `album:#42` is an ID, `album:"#42"` is a name; `album:unfiled` means unassigned, `album:"unfiled"` is a literal name.
- Repeated `format`/`album` fields merge; a repeated scalar field is an error even with the same value; unknown fields and partially invalid queries fail loudly instead of being dropped.

## Compatibility aliases

A few Lsky-inspired spellings normalize to this grammar:

| Alias | Canonical |
| --- | --- |
| `extension:jpg,png` | `format:jpg,png` |
| `is:public` / `is:private` | `visibility:public` / `visibility:private` |
| `order:earliest` | `sort:oldest` |
| `order:utmost` / `order:least` | `sort:size-desc` / `sort:size-asc` |
| `order:created_at` / `order:created_at-asc` | `sort:newest` / `sort:oldest` |

These are a fixed set, not a general Lsky compatibility language. `name:` is not a filename qualifier, and `$travel` is ordinary text.

## Protocol and limits

Queries run via `GET /api/images?qv=1&q=...&tz=Asia%2FShanghai&page=1&size=20` (album detail pages use `/api/albums/{id}/images` with the same parameters). `q` must be present but may be empty; `page` is 1–10000 and `size` is 20 / 50 / 100 only. `qv=1` cannot be mixed with the legacy keyword protocol.

- Raw and canonical queries must both fit 4096 UTF-8 bytes; an oversized canonical form returns `QUERY_TOO_COMPLEX` instead of executing truncated.
- At most 32 tokens, 16 raw plain terms (8 distinct), 200 code points per text value.
- The album boundary of a detail page is verified independently: clearing or changing the query never widens past that album.
- Successful responses return the canonical query and applied albums in `data.search` so the frontend can verify the filter actually applied; renaming an album does not break saved links (canonical references use album IDs).
