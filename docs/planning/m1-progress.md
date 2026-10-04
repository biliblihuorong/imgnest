# M1 progress — plan: docs/superpowers/plans/2026-10-04-m1-foundation.md

User approved M1 and parallel execution on 2026-10-04.

Execution: native tools plus three workers with disjoint file ownership; parent owns Git, dependencies, developer environment, HTTP, CLI, integration, and final review. Missing upstream workflow helpers are not invoked.

## Pre-flight and decisions

- Tasks 1→2→3: config.Database is the shared database opening contract; use the plan's exact field types.
- Tasks 3→4/5: model types are shared; domain sentinel errors live in internal/model and are aliased by service so repo never imports service.
- Tasks 4/5→6/7: service method signatures and DTOs match the accepted plan; constructors use context first.
- Tasks 6→7: Health is a context callback, allowing HTTP to check database health without importing repo.
- Ruling: user requested parallel execution after reviewing the plan — use this harness's workers and ledger rather than absent subagent-driven-development scripts — cost if wrong: execution bookkeeping changes, no product behavior change.
- Ruling: shared business sentinels are defined in model and re-exported by service — prevents repo→service reverse imports — cost if wrong: sentinel declaration locations can move without changing behavior.

## Tasks

1. Backend/environment: implementation complete; root CLI RED→GREEN, Go dev image and PG test server running.
2. Configuration: complete; worker RED→GREEN, final config race/coverage exit 0, 90.3% statements.
3. Database/migrations: complete; both drivers RED→GREEN, migration race pass, 79.0% statements.
4. User service/repository: complete; both drivers tested, unique/admin concurrency and atomic password/revoke rollback covered.
5. Token service/repository: complete; both drivers tested, hashes/expiry/ownership/touch/revoke/context covered.
6. Native HTTP/OpenAPI: complete; protocol tests and real SQLite/PG HTTP lifecycle passed. OpenAPI YAML and all local references checked.
7. CLI/lifecycle: complete; migrate/init-admin/reset/serve and stdin cancellation tests passed. Actual binary process smoke passed on SQLite and PG, including restart persistence.
8. Complete verification/review/docs: in progress; latest complete lint exit 0, final suite/race and independent review pending.

## Verification

Results are appended only after actual runs.

- Task 1 RED: `go test -mod=mod ./internal/cli -run TestExecute -v` found missing implementation; initial implementation then failed `TestExecuteUnknownCommand` (unknown args silently succeeded). Args validation fixed, final same tests exit 0.
- Task 2 RED: missing Load/Database followed by behavioral declaration-stub failures; final `go test -mod=readonly -race -cover ./internal/config -v` exit 0, 90.3%.
- Task 3 RED: Open and migration stubs failed on both drivers; final `go test -mod=readonly -race -cover ./internal/migrate ./internal/repo -count=1` exit 0; migration79.0%, repo76.3%. Rollback/checksum/unknown version/concurrency tested.
- Tasks 4/5 initial service two-driver suite exit 0, 87.0% statements. Earlier race snapshot passed in157.648s but preceded final password fixes; it is NOT claimed as final verification.
- Token ID regression: real SQLite and PG returned technical errors for MaxUint64; `TestOutOfRangeIDsAreAbsent` RED→GREEN after checking signed BIGINT range at repo boundary.
- Password regression: bcrypt comparison accepted 72-byte password plus suffix; overlong login/current-input tests RED→GREEN after explicit <=72 checks.
- Legacy regression: real8-byte bcrypt credentials failed after creation rules were reused for comparisons; VerifyLegacyShortPassword/ChangePasswordWithLegacyShortCurrentPassword RED→GREEN after separating new-password limits from credential validation.
- Task 6 RED: missing HTTP implementation; final `go test -mod=readonly ./internal/http/... -v` exit 0. Panic recovery leaked an injected private payload; TestPanicRecoveryAvoidsCredentialLogging RED→GREEN with custom middleware that logs only safe classifications.
- `go test -mod=readonly ./internal/http/... -run TestRealDatabaseHTTPAuthLifecycle -v` exit 0, both sqlite/postgres executed; login/create/list/revoke/change password/logout verified.
- Task 7 RED: missing commands/helpers; final `go test -mod=readonly ./internal/cli -v` exit0. Admin stdin cancellation initially remained blocked; TestAdminCancelledStdinReturns RED→GREEN after context-aware waiting.
- `go test -mod=readonly ./internal/cli -run TestActualProcessSmoke -count=1 -v` exit0; actual binary started both drivers, health succeeded, login survived a process restart, api tokens/revoke/logout behaved correctly.
- Module `tidy`/`verify` succeeded and binary build exit0. Exact direct pins preserved; actual pgx5.10.0/sqlite1.14.22 remarks corrected, testify remains indirect.
- Locked golangci-lint2.14.0 first exposed formatting/comments/context order and targeted test-fixture warnings; subsequent full lint exit0/0issues. Only justified test-fixture G101 and test-owned no-shell subprocess G204 suppressions remain.

## Clarifications and bounded choices

- Ruling: PostgreSQL without explicit DSN fails configuration — a SQLite filename is not a valid PG default — cost if wrong: an operator must provide an explicit DSN.
- Ruling: new/replacement passwords12–72 bytes, existing credential comparisons nonempty<=72 — preserve the agreed legacy bcrypt compatibility without suffix acceptance — cost if wrong: policy requires adjusting credentials validation and docs together.
- Ruling: returned/generated database IDs use signed BIGINT range though Go fields are uint64; out-of-range lookups are absent — avoid driver encoding errors for untrusted identifiers — cost if wrong: ID schemas and validation need coordinated changes.
- Ruling: CLI stdin cancellation returns promptly without closing a caller-owned Reader; a blocked arbitrary Reader goroutine may remain until one-shot process exit — avoids closing global stdin or commandeering caller resources — cost if wrong: long-lived embedders must supply an interruptible Reader.
- Development PostgreSQL uses isolated-network trust authentication and no host port; it is a test service, not a production deployment recipe.

## Independent review

Pending fresh reviewer over the complete feature branch. No merge, push, release tag or old Lsky data mutation has occurred.
