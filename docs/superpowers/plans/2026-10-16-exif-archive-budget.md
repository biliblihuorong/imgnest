# EXIF ISOBMFF archive repair

The user authorized delegated implementation, regressions and Pro integration.
Keep the preceding four product fixes and all existing user changes intact.

## Design

Use the existing version-1 raw archive with an additive optional `item_ids`
field. Group only equal kinds with exactly equal ordered absolute extents;
copy their data once and retain all IDs. Different kinds, source locations,
extent order or segmentation remain independent. Repeated and overlapping
extents inside an item preserve concatenation semantics and count in full.

Preflight every unique payload before allocation/copy/parse. The production
budget is 20MiB, matching the existing source cap; JSON/base64 overhead is
separate. Reject over-budget metadata as ErrInvalidMetadata, with no partial
archive. Use a private budget helper for small boundary tests, without a new
public API or runtime switch. Process IDs in ascending order and preserve
source-offset output order with deterministic ties.

## Work

- [x] Core agent: TDD unit regressions and minimal production implementation
  in internal/exif/isobmff.go and rawBlock's additive item_ids field.
- [x] Native agent: permanent tiny AVIF/HEIC native Probe → Extract controls
  for normal, separate and shared extents; preserve source bytes and fields.
- [x] Parent: update spec and image-pipeline Skill/reference consistently.
- [x] Integration: pinned Go/vips full vet, lint, race, SQLite/PostgreSQL/MinIO;
  Pro temporary workspace with repaired core, binary and packaged SPA smoke.
- [x] Review: OCR delegate preview/rule plus a fresh independent reviewer;
  inspect each changed file, regression and compatibility implication.
- [x] Delivery: integrity hashes, logs/report, cache size and cleanup of only
  task-created database/object-store containers and network.

Fixtures stay tiny (at most two or three metadata items, at most four extents
per unit-test item); exercise reduced budget boundaries rather than large
inputs or resource exhaustion. Preserve dependencies, released migrations,
privacy boundaries and lossless scrubbing. Pro's official pin remains until
the core fix is published; local workspace validation must be reported as such.

Evidence: .review/exif-fix-2026-10-16/. No commit or publication is requested.
