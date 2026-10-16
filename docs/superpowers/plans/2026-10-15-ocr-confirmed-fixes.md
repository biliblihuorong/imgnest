# OCR confirmed defect repair plan

**Goal:** Fix the four confirmed product defects from the October 15 OCR review,
ship regressions, and verify the remaining ISOBMFF metadata-budget risk.

**Execution:** The user explicitly chose delegation and authorized implementation.
Use the native collaboration tools for independent backend storage, frontend,
and EXIF verification tasks; the parent implements group-reference consistency,
integrates changes, runs the complete gates, and performs OCR delegation review.

**Architecture:** Preserve `http → service → repo/storage/imaging`. Storage
relocation and upload reservation must share a database consistency boundary;
group deletion and default-group assignment must recheck references in their
transactions. Frontend state uses existing Vue events and the shared site store.

**Spec:** `docs/spec.md`, `docs/versions.md`, project pipeline and Lsky Skills;
audit `.review/open-code-review-2026-10-15/REPORT.md` and `RECHECK.md`.

## Constraints

- Preserve all pre-existing website/document changes, business data, dependencies,
  released migrations, and existing build caches.
- Go 1.27.1, Node 24.21.0, pnpm 12.9.1, libvips 8.18.6; use the existing dev image.
- SQLite and PostgreSQL must both preserve the consistency guarantees. A process
  mutex or an extra service-level precheck alone does not resolve either race.
- No credential or GPS logging; keep public native/v1 envelopes unchanged.
- Write a regression, observe its expected failure, implement, and record green.
- EXIF verification uses tiny, bounded metadata samples and never resource
  exhaustion. Keep native decoder acceptance separate from extraction behavior.

## Review focus

1. Upload reads old backend before relocation, reserves afterward: reject before
   object writes; a pending/active image must prevent relocation atomically.
2. Credential rotation and display/base URL edits must remain possible when
   images exist, without allowing old configuration to overwrite new location.
3. Both orders of default-group assignment/deletion must leave valid settings;
   arbitrary persistence errors cannot count as a successful regression result.
4. Empty/error cover pickers must cancel via X, Escape, and mask; cancellation
   preserves unsaved album edits and normal selection remains functional.
5. Saved site flags/title must reach shared consumers immediately; failed saves
   must preserve old shared state. Settings refresh must not restore stale data.

## Tasks

- [x] Storage: implement real-repository race regressions and transaction guards
  in `internal/repo/storage.go`, upload reservations and their service plumbing;
  prove stale reservations are rejected and live images block relocation.
- [x] Groups: add real SQLite/PostgreSQL regressions, lock/revalidate group
  references in `internal/repo/settings.go` and `internal/repo/admin.go`, preserve
  business error mapping and structural group protection.
- [x] Frontend: regress `AlbumCoverPicker` / `AlbumFormModal` cancellation and
  `SettingsView` / shared site refresh, then implement minimal event/store fixes.
- [x] EXIF: verify a bounded shared-metadata sample with pinned native Probe and
  extraction; report accepted/rejected behavior and revise audit classification.
- [x] Integration: full Go race/vet, PostgreSQL/MinIO, frontend typecheck/lint/test/
  build, Pro against the local repaired core, binary build and CI wiring checks.
- [x] Review: run OCR delegate preview/rule on task changes, independently inspect
  each changed file and regression, resolve any findings, record final results.

Evidence lives in `.review/ocr-fixes-2026-10-15/`. Do not commit or publish unrelated
user changes; report the Pro dependency pin state explicitly at delivery.
