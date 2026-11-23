# Account Watermark Entry Implementation Plan

> Execute inline with executing-plans; one fresh branch review at the end.

**Goal:** Give licensed Pro users an independent “我的水印” account entry.
**User-approved design:** Account tabs expose each visible personal card by its server-provided title; watermarks appear at `/account/plugins/watermark`. Profile/security contains only profile and password. Free-group users see the group-control notice. Ordinary/unlicensed builds have no extra tab; direct unavailable links show a neutral unavailable state.
**Architecture:** AccountPage provides account-scoped plugin data to navigation and the selected card through typed Vue context. A composable owns loading/cancellation and reset on account/group changes. The thin route view selects one card. No paid plugin name is hardcoded in the core. No global settings store is introduced.
**Stack:** Existing pinned Vue/Router/Naive UI; no dependencies, API or database changes.

## Review focus
- Stale account responses and Blob URLs must not reappear after switching accounts, groups or routes.
- Navigation and card share one request within an account page.
- Only the selected card renders; unavailable cards expose no editable form.
- All account routes keep the account sidebar highlight and work on direct refresh.
- Mobile tabs wrap without horizontal overflow.

## Task 1: Account entry and shared context
- [x] Add failing profile/nav and router regressions; observe RED.
- [x] Extract useUserPlugins and typed injection context; update AccountPage and focused UserPluginSettings.
- [x] Add thin AccountPluginSettingsView and route; remove settings card from profile page.
- [x] Test readonly/unavailable views, account/route switching, load failures/retry and single request; run affected tests.

## Task 2: Verification and delivery
- [x] Update user/developer docs, labels and desktop/mobile browser fixtures.
- [x] Run frontend typechecks/lint/full tests/build and browser regressions.
- [x] Fresh read-only branch review; no blockers found.
- [ ] Publish core PR.
- [ ] Pin Pro to public core commit; run Pro tests and release/smoke, publish paired PR.

Authorization: User explicitly selected an independent “我的水印” entry. Existing authorization covers publishing paired feature PRs in these same repositories; merging these new PRs is not part of this change.
