# ImageNest Vben migration ledger

Plan: Library taskbook v2, approved by user; implementation resumed 2026-10-05 02:46 UTC after actual M5 submission and explicit start instruction.

## Source and architecture
- Upstream main and feat/m5-albums-gallery: d19323d2714f9d050ffcde8221ef2738e3e98e53
- Full Git tree verified: 77069ae372723ae46c4edc21ee83b96415f52dde (590 files)
- Local snapshot b67f5c9 is a synthetic provenance snapshot, not upstream Git history. No remote configured.
- Original web/ preserved from that exact M5 commit. New independent application web-vben/ has its own package, workspace, lockfile and dist.
- Previous paused in-place draft remains outside this working directory for recovery; only selected source was copied into web-vben.
- Vben MIT runtime source is pinned to50f4ede309d4450c7dd417399cb8d5c02346d2d2. Source manifest retained under web-vben/vendor/vben.

## Component boundaries
- App/bootstrap: one Pinia, same ImageNest authentication store, real Vben UI projections, i18n and Naive providers
- AppLayout/AuthLayout: actual Vben shell, role menus, branding; no demo auth flows
- Media domain: upload queue, images/details/trash, albums/gallery and authorized preview composable
- Account domain: tokens/password; separate real-metric dashboard domain
- Admin domain: six preserved management forms with reactive translation and action guards
- CAPTCHA: independent settings/provider service and verified native request integration; activation deferred to explicit operator action
- Build: compile-time legacy/Vben asset selection; old frontend hash guard

## Verified progress
- 02:50: M5 changed blobs and complete source tree match upstream; original frontend protected.
- 02:53: frozen Vben workspace dependency installation succeeded with exact Node24.21.0/pnpm12.9.1.
- 02:56: structured API error localization3/3 passed after observed failing tests.
- 02:59: login9/9 passed, including hidden registration entry, role home, language retention and safe redirect.
- 03:02: navigation/session9/9 passed after observed M5/public-menu failures.
- 03:07: auth7/7 passed including immediate sensitive-preview clear event.
- 03:08: user media126/126 focused tests, owned lint/format passed; aggregate and browser checks pending.

## Rulings and validation limits
- Public album flag in M5 is metadata only: no shareable album endpoint or implied public album access added.
- Dashboard shows exact current/native counts and charged storage only; unsupported historical/today/frequency/uploaders metrics remain absent.
- M5 cover-zero and album counter defects reproduced with SQLite migrations and tagged Go repository tests. User approved exactly these two minimal fixes; PostgreSQL verification and fixes in progress.
- Ordinary Go repo suite passed, but excluded integration-tagged tests; it does NOT prove the M5 regressions pass.
- No remote pushes, merges, deployment, provider account setup, credential entry or CAPTCHA activation.

## Integration and independent review follow-through
- 03:21: core26 tests, both TypeScript configurations and Vben production build passed on the pre-final snapshot.
- 03:23–03:49: execution was interrupted; no progress was claimed for this interval. Work resumed from preserved files.
- 04:06: real BasicLayout/route-content regression proves language changes preserve the active form and queued state; opt-out keeps upstream default behavior for other consumers.
- 04:07: authenticated admin thumbnails use the same abortable blob lifecycle as media/album views; 23 admin/settings tests passed.
- 04:12: demotion now leaves the open administrative route, with mounted-view disposal regression5/5 passed.
- 04:16: pending upload/session transmission, stale401, slow-login account replacement, late logout, and dropped token-list refresh findings all have focused RED→GREEN coverage.
- CAPTCHA defaults disabled. Real credentials/accounts/provider activation were never used; mock/provider contract checks do not attest production deployment-domain verification.
- PostgreSQL18.6 executed actual M5 migration SQL: both original defects reproduced, and8 patched-semantics assertions passed. Full Go→PostgreSQL/concurrent-runtime integration remains unexecuted because its local Unix socket could not be created.
- CUA local preview reported ERR_BLOCKED_BY_CLIENT. Repository-prescribed isolated Chromium E2E could start both local fixture servers, but Chromium's private process socket returned EPERM. Supported escalated retry failed at runtime mount setup; official headless-shell installation yielded an invalid archive. No security settings or public tunnel were changed. No browser/visual/accessibility pass or screenshot is claimed.
- Reproduction-ready local fixture E2E scripts are retained in web-vben/e2e with their unexecuted status.

## Final executable and source checks
- Final new frontend:52 suites/504 tests, both TypeScript configurations, ESLint and production build passed. Added final immediate session-disposal and stale logout-view navigation regressions before this rerun.
- Final backend:15 tested Go packages passed race suite; Vben CLI subprocess tests passed; pinned golangci-lint reports0 issues.
- Real binary HTTP switch passed legacy→vben→legacy with exact asset hashes, shared isolated SQLite/config, disabled CAPTCHA login, preserved session/schema and graceful cleanup. Repeated after the final frontend changes.
- Remaining acceptance gaps are explicitly limited to browser/visual/accessibility rendering, live provider/domain activation and external PostgreSQL/MinIO integrations; no corresponding success claims or screenshots are supplied.
