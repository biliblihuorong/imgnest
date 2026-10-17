# Sign-in protection

Three features that work together: Cloudflare Turnstile verification, SMTP with email codes, and password reset.

## Turnstile verification

Off by default. When enabled it protects web login, registration and code sending:

- Configure it in one step under Admin → Settings → Registration and sign-in: enter the Site Key and Secret (an empty Secret reuses the saved one), tick enable, pass one real challenge on the page, and the server activates immediately after verifying it. Disabling needs no challenge. Allowed hostnames default to the current host and sit under Advanced.
- Turnstile is the only provider. Hostnames must be exact lowercase fully qualified names, at most ten — no wildcards, ports or IPs.
- The Secret is write-only and encrypted with `security.master_key` — **back up the master key before rotating it**, otherwise saved secrets become undecryptable (see [Configuration](./configuration)).
- The older draft / test / activation endpoints remain available for API users who want the stricter flow.
- Clients that send no `captcha_token` simply fail while verification is on — there is no bypass. `/api/v1/tokens` (email + password for a token) is not covered by the challenge, so PicGo and other non-interactive uploads are unaffected.

## SMTP and email codes

Configure SMTP under Admin → Settings → Mail; "send test" mails a test letter with the saved settings (to yourself by default, 3 per IP per minute).

| Setting | Notes |
| --- | --- |
| Encryption | `tls` (465) / `starttls` (587) / `none` (25, internal unauthenticated relays only) |
| Password | Write-only, encrypted with the master key; without a master key only unauthenticated relays can be saved |
| Failures | Reported as one of `connect` / `tls` / `auth` / `rejected` plus the numeric SMTP code; the server's raw reply is never echoed |

With "verify registrations by email" enabled (requires working SMTP):

- Registration must carry the 6-digit code from the email; the challenge (if enabled) moves to the code-sending step.
- Codes last 10 minutes, cannot be resent within 60 seconds, are void after 5 wrong attempts, and are deleted once used. Only the newest code per (purpose, email) is kept.
- A failed send voids the code, so no unreachable code is ever left behind.

## Password reset

When SMTP works, the login page gains "forgot password": `POST /api/auth/email-code` (`purpose=reset_password`) sends a code, and `POST /api/auth/password-reset` resets with it (3 per IP per minute).

Unregistered, disabled and cooling-down addresses all get the same success response, **never revealing whether the account exists**. A successful reset revokes every token of the account — all signed-in devices and PicGo tokens stop working and must sign in again.

## Rate limits at a glance

| Action | Limit |
| --- | --- |
| Login / register / send-code endpoints | 3 per IP per minute |
| Admin mail test | 3 per IP per minute |
| Code resend | 60-second cooldown |
| Code attempts | 5 wrong tries void the code |
