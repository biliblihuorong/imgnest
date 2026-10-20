# Account and usage

## Change your profile or password

- New layout: sidebar gear → Settings → Account. Classic layout: top-right user menu → Account settings. You can also open `/account/settings` on your own site.
- Edit Display name and click Save profile. Leaving it empty uses your username. Email, username and avatar provider are not editable in this form.
- Under Account security, enter the current password and confirm a new one, then click Change password. You must sign in again; all existing browser sessions and API Tokens are revoked, so clients such as PicGo need new tokens.
- Create or revoke API Tokens on Account & tokens (`/tokens`); see [Lsky API and PicGo](./lsky-api.md#get-a-token).

## View your usage and activity

Open Overview (`/dashboard`) from navigation for your image, album and storage totals. Account activity below shows recent logins, active sessions and API usage. Refresh data reloads the cards; failed cards offer a retry.

Recycle-bin totals differ from quota usage: quota is released while recycled objects still occupy storage. See [Privacy and recycle bin](./privacy-and-trash.md#recycle-bin). API usage uses UTC days; the latest minute may not yet be included.

## Administrator views

Administration → Site overview (`/admin/overview`) shows site-wide totals. Users → Details shows that user’s activity. All images (`/admin/images`) manages every user’s images. Ordinary accounts do not see these entries.

## Extension settings

Administrators can open Administration → Site settings → Plugins (`/admin/settings/plugins`). Only plugins compiled into the running program and exposing configuration appear. An empty page means there are no configurable plugins; it is not an installer. Building extensions is a developer workflow described in the repository.
