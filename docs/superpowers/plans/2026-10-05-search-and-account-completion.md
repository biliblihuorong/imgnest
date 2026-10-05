# Search and Account Completion Implementation Plan

> For agentic workers: follow the repository's test-driven development and verification skills. The user approved implementation and publication on a new branch; merging and deployment are outside this task.

**Goal:** Correct album empty-state actions, implement unified image search v1.1 in both personal image lists, and complete administrator account creation/editing without changing self-service identity rules.

**Architecture:** Keep the existing HTTP → service → repository layering and shared ImageLibrary. Add a versioned search protocol alongside legacy parameters. Keep account permissions as existing role plus group policy, with atomic lifecycle safeguards.

**Tech Stack:** Existing locked Go 1.27.1, SQLite/PostgreSQL, Vue 3, Vben/Naive UI, TypeScript, Vitest. No dependency upgrades.

**Spec:** ImageNest unified-search TXT version 1.1, approved 2026-10-05; existing `docs/spec.md`, `docs/openapi.yaml`, and the account scope documented below.

## Global Constraints

- Baseline: `f25a448a1078663095a7450994b4de4348f704f0` on `feat/vben-ui-migration`.
- Preserve upload feedback, copy menus, preview/lightbox, avatars, storage corrections, and frozen `web/` sources.
- Do not operate live accounts, live storage, or production migrations. Use isolated test fixtures.
- Search qv=1 must filter and count the full authorized dataset before pagination; legacy requests remain compatible.
- New search dates refer to upload `created_at`, use validated IANA time zones, and follow [after, before) day boundaries.
- Usernames and emails remain read-only in personal settings; display names are optional, non-unique, and fall back to username.
- No new granular permission system, invitation/email workflow, audit subsystem, merge, or deployment.

## Review Focus

- Album scope survives clear, cross-album terms, back/forward, reused route components, and account changes.
- Search parsers agree on quoted lists, aliases, Unicode spans/normalization, exact byte sizes, and malformed queries.
- Count, rows, suggestions, and error candidates share the same owner boundary, including OR clauses and inaccessible albums.
- Concurrent account edits cannot disable/demote the last active administrator or resurrect invalidated login credentials.
- Delayed requests and cancel/reopen flows cannot overwrite a newer session, draft, selection, or modal.

## Task 1: Album empty states

Files: AlbumsView.vue, useAlbums.ts, AlbumsView.test.ts, album translations.

- [x] Add failing tests for no-album create CTA, existing empty album card, invalid last-page recovery, and inconsistent empty page.
- [x] Reuse AlbumFormModal for no-album CTA; distinguish total=0 from an empty current page.
- [x] Recover a disappeared final page once without unbounded request loops or stale writes.
- [x] Run the focused tests and all frontend regressions.

## Task 2: Search protocol and persistence

Files: new internal/searchquery package and shared JSON vectors; native image/album handlers; image/album services and repositories; model fields; versioned migrations.

- [x] Write parser/serializer vectors covering TXT T01–T23 and E01–E14 before implementation.
- [x] Implement exact AST, aliases, UTF-16 diagnostics, normalization, fixed-point sizes, and strict limits.
- [x] Implement qv=1, owner-scoped album resolution, IANA day conversion, SQL predicates, stable sorting, accurate count, and response metadata.
- [x] Add /api/albums/{id}/images as an independently checked fixed album boundary and safe album suggestions.
- [x] Implement restartable bounded normalization backfill only through migration, with readiness checks and rollback documentation.
- [x] Verify SQLite/PostgreSQL, literal LIKE escaping, cross-page hits, authorization, legacy compatibility, migration recovery, and query performance.

## Task 3: Shared search interface

Files: ImageLibrary, useImageLibrary, dedicated query-state/input/help/chips/suggestion components, pure TS parser, API wrappers, locales, and tests.

- [x] Share parser vectors with Go; reject invalid input without partial queries.
- [x] Replace the old independent filtering controls with a single draft query, chips, completion, clear, submit, and accessible ? help.
- [x] Keep applied query/result separate from drafts; synchronize qv/q/tz/page/size with browser history and canonical server responses.
- [x] Verify IME, keyboard completion, focus restoration, localized messages, stale requests, account changes, and existing image actions in component/API tests.
- [ ] Verify actual mobile/desktop layout and browser accessibility; blocked by the available browser runtime, not substituted by component tests.

## Task 4: Account administration

Files: admin account service/repository/handlers, authentication proof checks, admin UI/form/API/translations, and tests.

- [x] Add tests for authenticated administrator creation and edits, duplicate names/emails, guest-group refusal, self-disable prevention, and last-admin concurrency.
- [x] Reuse existing username/email/password validation and bcrypt for initial passwords; never accept a password in account PATCH.
- [x] Support username/email/display_name/role/status/group_id with strict whitelists, atomic writes, safe error mapping, and token invalidation for effective identity/authorization changes.
- [x] Pin login credential proof to account identity/authorization so an in-flight login cannot bypass an administrative revocation.
- [x] Filter invalid group options, update self-session behavior, and reject missing/null display_name in self-profile PATCH while retaining explicit empty-string clearing.

## Task 5: Integration, verification, publication

- [x] Update OpenAPI and regenerate only web-vben schema types; reconcile spec and migration documentation.
- [x] Run backend tests/race/lint, frontend tests/typecheck/lint/build, frozen legacy-source checks, and real-API regressions; browser/S3 limits documented separately.
- [x] Record every passed, failed, skipped, and unrun check with commands; do not infer browser or database coverage from unit tests.
- [x] Review the complete diff and confirm the source branch has not advanced before publication.
- [ ] Publish an independent feature branch, verify remote commit/tree, and inspect available CI for that exact commit.
