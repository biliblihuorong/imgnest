# Native authentication CAPTCHA

This implementation adds a disabled-by-default Cloudflare Turnstile policy for native web login and registration. It does not create a Cloudflare account, provision a widget, install real credentials, or activate protection. Real provider/browser validation remains a release check on the intended deployment domain.

## Scope and compatibility

- `POST /api/auth/login` verifies action `login` before password verification and web-token issuance when enabled
- `POST /api/auth/register` verifies action `register` before creating an account when registration is open; closed registration stays closed without consuming a challenge
- The existing independent three-request-per-IP-per-minute limits run before provider verification; trusted-proxy configuration remains unchanged
- `/api/v1/tokens` retains its current email/password contract and rate limit. CAPTCHA does **not** protect all password authentication while that endpoint is enabled
- PicGo and existing bearer-token uploads remain noninteractive and unchanged
- The legacy `web/` frontend was removed on 2026-10-07. A client that does not send `captcha_token` still fails closed when CAPTCHA is enabled; the `acknowledge_legacy_incompatibility` activation field is kept for API compatibility. A header, query parameter, username, role, or selected frontend never bypasses verification

A release operator must explicitly decide the legacy rollback policy before enabling protection: either authorize a separate legacy widget adaptation, or accept that rollback requires an explicit authenticated/operator decision to disable native CAPTCHA. The current code does not choose a downgrade automatically. Activation requires acknowledgement of both legacy incompatibility and the unprotected v1 password surface.

## Configuration contract

The exact request and response schemas are in [`openapi.yaml`](openapi.yaml).

| Endpoint | Access | Purpose |
| --- | --- | --- |
| `GET /api/auth/captcha` | Public | Minimal active `enabled`, `provider`, `site_key`, `version` |
| `GET /api/admin/captcha` | Enabled administrator | Safe active/draft metadata and master-key availability |
| `PUT /api/admin/captcha/draft` | Enabled administrator | Save an encrypted candidate without changing active protection |
| `POST /api/admin/captcha/test` | Enabled administrator, 3/IP/min | Consume a fresh candidate token for `login` or `register` |
| `POST /api/admin/captcha/activation` | Enabled administrator | Promote a tested candidate or explicitly disable protection |

Every mutation requires the most recently returned top-level `version` as `expected_version`. The response contains the next safe admin snapshot. A stale request returns HTTP 409/code `30011`; reload and review before retrying. Candidate configuration has its own `draft.version`; the top-level version also advances when test results or activation change.

The existing `settings` table stores one `captcha_config` JSON snapshot. Compare-and-swap is an atomic insert/update, including active configuration, candidate, encrypted credentials, and test results. No released migration is edited or schema migration needed. Each authentication request reads one immutable snapshot, so an in-flight verification never mixes site/provider/secret/hostnames from different updates.

Only `turnstile` is accepted. Hostnames must be unique lowercase fully qualified DNS names, at most ten, without a URL scheme, path, port, wildcard, IP address, or trailing dot. The response hostname must be an exact configured match. The site key is public; the secret is write-only. Secret omission reuses the current candidate's secret, or the active secret if no candidate exists, only for the same provider. A supplied empty secret is invalid. Explicit `clear_secret=true` is allowed only while disabled, must omit `secret`, and also discards the retained active copy.

The deployment uses the existing `security.master_key` / `IMGNEST_SECURITY_MASTER_KEY` (base64 of 32 bytes) and AES-GCM codec. The provider name is included in authenticated encryption context. Preserve the master key alongside private database backups; changing it without re-encrypting existing credentials makes them unreadable. Neither plaintext nor encrypted values are returned through public, CAPTCHA-admin, or general-admin settings responses. The secret, token, email, and password are excluded from provider diagnostics and application errors.

## Candidate verification and activation

1. Keep a valid administrator session open on the Vben deployment and ensure an authorized operator can access its server and private backups
2. Independently provision a production widget for the exact target hostnames, review provider terms/privacy requirements, and supply its public site key plus write-only secret to the candidate form. Production settings reject known official test site keys and secrets
3. Generate a fresh widget token for `login` and test it against the saved candidate. Use the returned top-level version for the next request
4. Generate a different fresh token for `register` and test it. Both results must be at most 15 minutes old when activation is requested. Saving any candidate edit invalidates both tests. Concurrent edits cannot inherit an in-flight success
5. Review both compatibility consequences, acknowledge them explicitly, and activate using the newest version. Existing administrator sessions are retained
6. From an independent session, verify native login and direct registration behavior, role routing, the closed registration state, and an intentional bad/expired/reused token. Record the tested application build, domain, timestamp, result, and operator without recording credentials or tokens

Saving a new candidate while enabled leaves active protection unchanged. A successful activation swaps the entire candidate at once. If the provider is unavailable, the database cannot be read, or the secret cannot be decrypted, native protected authentication remains blocked. The server never automatically disables protection or falls back to password-only authentication.

## Provider protocol and failure behavior

The adapter posts only `secret` and `response` to the fixed HTTPS endpoint `https://challenges.cloudflare.com/turnstile/v0/siteverify`. It never sends account fields, custom business data, or client IP. It refuses redirects and has a five-second overall deadline, two-second connect/TLS bounds, a three-second response-header timeout, and a 16 KiB response-body budget. There is one attempt and no automatic retry after an ambiguous result. A submission failure requires a fresh frontend challenge.

Accepted proof requires success, no provider errors, the expected action, an exact allowed hostname, and a nonfuture challenge timestamp within 300 seconds. Cloudflare enforces single use. The token input budget is 2048 bytes. These protocol limits were checked against [Cloudflare server-side validation](https://developers.cloudflare.com/turnstile/get-started/server-side-validation/) and [test-key documentation](https://developers.cloudflare.com/turnstile/troubleshooting/testing/) on 2026-10-05.

| HTTP / native code | Meaning | Client next step |
| --- | --- | --- |
| 422 / `30010` | Missing, oversized, expired, replayed, rejected, or mismatched proof | Render/reset and complete a fresh challenge |
| 409 / `30011` | Configuration version changed | Reload configuration and review the current state |
| 409 / `30012` | Candidate/tests/activation acknowledgements incomplete or expired | Reload, complete both tests and acknowledgements |
| 503 / `50004` | Provider/network/configuration/encryption unavailable | Keep protection in force; retry when restored |
| 429 / `30003` | Existing route IP limit exceeded | Wait for the request window before a fresh attempt |

## Operator recovery

Prefer restoring the provider, original master key, or valid configuration while keeping protection enabled. An existing authenticated administrator can read the newest version and explicitly disable through the activation endpoint even when the master key is absent or the provider is down, provided the stored snapshot is structurally valid. Do not expose an anonymous disable URL or a frontend/header bypass.

If every administrator is locked out and the operator has explicitly authorized the security change:

1. Restrict external ingress and stop **all** application replicas so no request or competing writer remains in flight
2. Take a private database backup with its matching master key; do not print/export the `captcha_config` value into a ticket or ordinary logs
3. Record the operator, reason, UTC time, build, approved security consequence, and old version in the deployment audit record. Inspect only `version` and `enabled`, not the secret-bearing row
4. In a database transaction, set only `enabled` to false and increment top-level `version`. Check that exactly one structurally valid row changed. Retain active/draft data so repair can be reviewed later. Example statements for a valid stored snapshot:

SQLite:

```sql
BEGIN IMMEDIATE;
UPDATE settings
SET value = json_set(value, '$.enabled', json('false'),
                     '$.version', json_extract(value, '$.version') + 1),
    updated_at = CURRENT_TIMESTAMP
WHERE key = 'captcha_config'
  AND json_valid(value)
  AND json_type(value, '$.version') = 'integer'
  AND json_extract(value, '$.version') BETWEEN 1 AND 9007199254740989;
SELECT changes();
COMMIT;
```

PostgreSQL:

```sql
BEGIN;
UPDATE settings
SET value = jsonb_set(
      jsonb_set(value, '{enabled}', 'false'::jsonb),
      '{version}', to_jsonb((value->>'version')::bigint + 1)),
    updated_at = CURRENT_TIMESTAMP
WHERE key = 'captcha_config'
  AND jsonb_typeof(value->'version') = 'number'
  AND (value->>'version')::bigint BETWEEN 1 AND 9007199254740989
RETURNING value->>'version' AS version, value->>'enabled' AS enabled;
COMMIT;
```

5. If zero rows change, validation fails, or the row is corrupt, roll back and restore a known valid private snapshot with operator review. Do not delete the setting or assume the policy became disabled
6. Restart one replica behind restricted ingress. Check `GET /api/auth/captcha` reports `enabled=false`, then verify an administrator can log in. Record the new version and recovery result; restart remaining replicas and restore ingress only after review
7. Repair and retest the candidate, then explicitly re-enable when the provider, domain, compatibility decision, and credentials are ready. Recovery itself does not attest either candidate action

The SQL examples are an offline operator procedure, not a network-accessible maintenance API. They do not grant permission to perform recovery, alter production settings, change v1 policy, or accept new provider terms.

## Verification boundaries

Automated adapter tests use an in-process HTTP transport, fabricated nonproduction identifiers, and deterministic timestamps. Service tests use the real AES-GCM codec and exercise fail-closed behavior, role checks, test expiry, candidate invalidation, stale/concurrent writes, and snapshot isolation. Repository CAS tests run with SQLite and with PostgreSQL when `IMGNEST_TEST_POSTGRES_DSN` points to an isolated test database. Native handler tests cover strict JSON, errors, rate-limit ordering, public minimization, private access, registration state, and legacy payload compatibility while disabled.

Mock success is not a real Cloudflare integration result. Production widget checks and assistive-technology/browser checks on the deployment domain must be recorded separately before actual activation. No real keys or account were used by this implementation task.
