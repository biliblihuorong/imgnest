# M2 progress — plan: docs/superpowers/plans/2026-10-04-m2-image-core.md

User confirmed on 2026-10-04: private images only hide public display, direct links remain accessible; scrub failure rejects upload; quota sums unique cloud object bytes (original/WebP/cloud thumbnail). Parallel workflow preserved.

Baseline: M1 2f00a6f. Branch: feat/m2-image-core. Parent owns dependencies/environment/CLI/HTTP/services/integration/docs/Git; workers own persistence, imaging+metadata, and paths+storage. No old Lsky data, remote pushes or new unapproved dependencies.

## Pre-flight

- Shared DTOs and value types follow M2 plan; receipts persist in model, not storage implementation types.
- Current opaque TokenSubject is carried through upload and lifecycle writes; repository rechecks source credential under the existing user lock.
- Pending image rows hold path/amount; SUM pending/restore values under user lock avoids a second cached capacity counter.
- Original0001 remains immutable; new0002 only; all Image states reserve unique(storage_id,path).
- Single process resumes unfinished IO before serving; no TTL-only release while an old writer might still exist.
- vipsgen GetBlob is avoided based on source ownership findings; raw metadata comes from containers; exact pinned versions remain.

## Tasks

1. Pinned environment/contracts: in progress.
2. Paths/local driver: pending worker.
3. S3/version cleanup: pending worker.
4. Schema/repository reservations: pending worker.
5. Imaging/WebP/thumb: pending worker.
6. EXIF/XMP/archive/scrub: pending worker.
7. Encrypted provisioning: pending parent.
8. Upload/compensation: pending parent.
9. Native image API/local serving: pending parent.
10. Trash/restore/purge: pending worker+parent.
11. Actual process/failure integration: pending.
12. Full verification/review/docs: pending.

Results and deviations are recorded only after real runs.
