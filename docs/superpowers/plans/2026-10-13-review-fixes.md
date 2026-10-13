# ImgNest Review Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Resolve the ten confirmed findings from the 2026-10-13 ordinary and Pro review.

**Architecture:** Keep the shared core services and persistence interfaces. Correct TIFF archive allocation, atomic identity linking, and client session/request lifecycle. Build Pro against the same pinned core source checkout whose frontend is embedded; never change the module cache or copy core into Pro permanently.

**Tech Stack:** Go 1.27.1, libvips 8.18.6, Vue 3.5.43, Node 24.21.0, pnpm 12.9.1; existing locked dependencies only. Python standard library may orchestrate the build, as in existing core scripts.

**Spec:** `docs/spec.md`, `docs/versions.md`, `.review/IMGNEST_REVIEW_2026-10-13.md`; Pro also follows its `CLAUDE.md`.

## Global Constraints

- Preserve `http → service → repo / storage / imaging`; service depends on interfaces.
- Keep original-image privacy edits lossless and full private EXIF intact. No silent truncation.
- Native and Lsky v1 share services; keep v1 fields, units and owner boundaries.
- No new product dependencies, toolchain upgrades, production AutoMigrate or edits to published migrations.
- Credentials and sensitive metadata never enter logs. Keep main unchanged and use reviewable fix branches.
- Follow test-driven-development: reproduce failures before implementation, then run the relevant suites.

## Review Focus

- Same-offset TIFF values with different lengths and overlapping ranges must retain all bytes without repeated amplification.
- TIFF privacy cleaning must preserve pixels and DNG structural fields.
- Identity links must not survive a concurrent promotion, disable, email reassignment or password/authorization epoch change.
- A stale response or another tab's account change must not mix identities, previews or queued uploads.
- Pro releases must use the exact pinned core revision, real frontend assets and an unlicensed fallback; one client must not block others' SSO.

### Task 1: TIFF archive and privacy

**Files:** `internal/exif/tiff.go`, `internal/exif/processor_test.go` or focused TIFF tests. Synchronize image-pipeline docs only if contract changes.
**Interfaces:** Keep `Processor.Extract` and `Processor.Scrub` unchanged. Archive ranges carry exact source offsets and all metadata bytes.

- [x] Write actual Extract/Scrub regressions for overlapping values, same-offset longer values and ASCII CameraSerialNumber; assert bounded archive payload, complete byte preservation, stripped identity and unchanged valid image pixels.
- [x] Run the tests in the locked dev image; expected RED on archive size and serial retention.
- [x] Merge covered archive byte ranges before allocating, and strip the named serial field without rejecting harmless DNG structures.
- [x] Run the full EXIF/imaging suites; expected exit 0. Record RED/GREEN output and include fixes in a focused core commit.

### Task 2: Atomic external identity binding

**Files:** `internal/service/external_signin.go`, `internal/repo/identity.go`, `internal/service/external_signin_test.go`, `docs/development.md`.
**Interfaces:** Extend `IdentityRepository.LinkIdentity` to consume expected account proof (existing `model.TokenGrant`) and return the account verified under its lock. Pro uses only public Host and needs no signature change.

- [x] Write dual-database interleaving tests that alter role, email, status or credential state after lookup but before linking; assert no identity persists and no later sign-in can use it.
- [x] Run tests against current implementation; expected RED on persisted identity.
- [x] In the repository transaction lock the user, validate exact grant and non-admin role, then insert identity; use the locked returned user in the service.
- [x] Run service/repository and HTTP SSO tests against SQLite/PostgreSQL; expected exit 0. Document the binding transaction and commit.

### Task 3: Frontend request and session lifecycle

**Files:** `web-vben/src/api/client.ts`, an optional session module, `stores/auth.ts`, `router/index.ts`, album components and their tests.
**Interfaces:** API client, XHR, router and previews consume one current token source. Cross-tab replacement clears old account data before restoring the new identity. Album request results are applied only to the current album/query.

- [x] Write regressions for failed storage writes/removals, cross-tab A→B changes and pending old responses; real router/API must use the correct current identity.
- [x] Write deferred-response component tests for album mutation during navigation and query/page replacement during cover loading; expected RED on stuck saving or stale displayed results.
- [x] Implement unified session state, stale-response fencing and latest-query handling. Diagnose the Naive UI teardown exception without suppressing uncaught errors.
- [x] Run targeted RED/GREEN, then pnpm test, typecheck and lint in the pinned image; expected no unhandled exceptions and exit 0. Commit frontend fixes.

### Task 4: Pro SSO fallback and fairness

**Files:** `/home/shell/project/imgnest-pro/internal/sso/*`, `cmd/imgnest-pro/main.go` and tests.
**Interfaces:** Existing plugin routes and Host remain unchanged. Disabled plugins do not validate unused provider credentials. Commands that do not serve HTTP do not require SSO. Start limits are per client with bounded process memory.

- [x] Write tests for incomplete example config without license, non-serve core commands with unused SSO config, and one client's starts followed by another client's valid start; expected RED.
- [x] Apply authorization-aware config loading and fair bounded start allocation; preserve PKCE, nonce, cookie binding and single-use callbacks.
- [x] Run Pro SSO/config/license and command tests with race detection; expected exit 0. Update SSO instructions and commit.

### Task 5: Pro packaged frontend and core pin

**Files:** `/home/shell/project/imgnest-pro/scripts/build-release.py`, `Makefile`, `.github/workflows/ci.yml`, `README.md`, `go.mod`, `go.sum`, `cmd/imgnest-pro` process smoke tests.
**Interfaces:** Build resolves the core revision from Pro's pinned module, copies a matching Git checkout to temporary build space, builds its locked frontend, uses a temporary Go workspace for Pro + core, and emits a complete binary. `--core-dir` provides a matching local checkout for deterministic offline verification. Core dependency points at the published, reviewed fix commit; no permanent replace.

- [x] Add a real packaged-server test for `/healthz`, `/login`, `/auth/sso` and referenced assets; run against a standard current Pro binary, expected RED on absent HTML.
- [x] Implement exact-revision source preparation and release build, with toolchain checks and frontend-content checks before Go compilation. Add CI execution of the real binary smoke test.
- [ ] Update the existing core dependency after core tests/review, preserving every other locked version.
- [ ] Build Pro with the pinned fixed core and run process smoke plus Pro suites; expected GREEN. Describe release command and commit.

## Final verification and review

- [x] Ordinary Go vet and full `go test -mod=readonly -race -shuffle=on -count=1 -timeout 40m ./...` with fresh SQLite/PostgreSQL/MinIO test services and temporary Git safe-directory environment.
- [x] Frontend full test, typecheck, lint and build; CLI/frontend embed validation after build.
- [ ] Pro tests, release binary smoke and core dependency resolution without a committed workspace or replace.
- [x] Fresh independent code review of both complete diffs; fix important findings with reproducing tests.
- [ ] Create draft PRs for both fix branches, attach them to this chat, preserve main, and report validation plus remaining limitations.

## Core publication checkpoint

The checked steps passed locally on the fixed core before publication. Final core vet/race/shuffle, binary build and golangci-lint completed with zero errors; frontend passed 850 tests, both typechecks, lint (four existing warnings) and production build. Independent review findings were fixed with palette/ICC overlap and real cross-tab upload lifecycle regressions. The remaining Pro dependency, release and PR steps depend on publishing this core commit; their final evidence belongs with the Pro change.
