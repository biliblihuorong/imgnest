# Storage implementation contract

`Driver` owns object bytes and metadata; callers reserve canonical keys in the
database until all associated IO and compensation have finished. An `OwnerID`
is the stable image key, not an operation identifier or a credential.

## Local objects

Local objects are single atomic files at the canonical key. Their internal
format is the eight-byte magic `IMGNST01`, a four-byte big-endian JSON-header
length, the JSON `ObjectInfo`, then the exact original body. This keeps body and
ownership together across crashes. A temporary file is synced and installed
using an atomic, non-replacing hard link; an existing target is never replaced.

Always read via `Driver.Open`, which skips the header. `Stat.Size` and receipts
count body bytes only. Do not serve the storage directory using a raw file
server or assume these files are directly decodable images. Backups must retain
the complete stored files. The separate `data/thumbs` cache is raw WebP and must
not use this object format. Staging names are derived from SHA-256 of key, NUL,
and owner. Normal error and cancellation paths remove them; after an interrupted
process, `PurgeOwned` removes the exact key/owner staging file even when no
canonical object was installed. The caller must finish or fence old IO before
starting compensation, as required for its database operation reservation.

All local key operations use `os.Root`; absolute paths, dot segments, symlinks,
and internal staging names are rejected. Purge acts on the exact file only.

## S3-compatible objects

S3 bodies are raw image bytes. Metadata includes the stable owner and SHA-256.
Put uses `If-None-Match: *`, explicit content length, and SDK checksums only when
required. Unknown endpoints are not presumed to implement preconditions: before
first use the adapter writes tiny random probes, verifies conflicts preserve
existing bytes, and removes its probe versions. `Check` also exercises copying;
a storage connection test should call it before enabling a rule.

Copy stays server-side via multipart copy and conditional completion. This is
required because the pinned MinIO release does not enforce destination
preconditions on `CopyObject`. Conditional completion is probed independently.
In-progress multipart uploads are aborted on failures with a bounded cleanup
context. Copy retries reuse a target only when owner, size, and SHA-256 match.

`PurgeOwned` is the compensation operation: it lists exact-key versions and
deletes only matching-owner versions. Unowned deletion markers and foreign
history remain, so compensating a new write cannot reveal or destroy external
history. `PurgeImage` is the product's final destruction operation: it checks
every entity version's ownership before deleting anything, then removes all
verified versions and deletion markers, including pagination, and confirms that
the key has no remaining history. Any foreign history rejects the whole purge.
`PurgeAllVersions` is reserved for explicitly owned connection-test namespaces;
it must not be substituted for compensation or user image deletion.

A write error can occur after the provider committed. The caller must inspect
the returned receipt and durable ownership, and compensate all attempted keys.
No B2/COS/R2 account is implied tested by MinIO passing; endpoints lacking the
needed conditional-write, multipart, or version-listing capabilities fail
explicitly instead of receiving an unsafe fallback.
