# Extensions and Pro

The ImgNest core ships an in-process plugin interface; the paid ImgNest Pro builds enterprise features such as single sign-on on top of it without changing any free-edition behavior.

## What plugins can do

Plugins compile into the binary and mount under `/api/ext/{name}`. Capabilities the core exposes:

| Hook | Purpose |
| --- | --- |
| Login providers | Append third-party sign-in buttons to the login page; the plugin verifies the identity and the core handles account binding |
| Access guards | Apply custom allow / deny rules on public image routes (`/i/...`, thumbnails, gallery) |
| Events | Subscribe to upload, delete and other site events |
| Display transforms | Overlay watermarks and similar on the served WebP |

Each plugin's routes are isolated — none can shadow core routes or another plugin's. Plugin names allow `[a-z0-9-]` only, and plugin setting cards appear under Admin → Settings → Plugins.

Plugins currently ship with the Pro distribution; `extension/extension.go` in the core repository is the authoritative interface.

## ImgNest Pro

Pro is a separate repository, [imgnest-pro](https://github.com/biliblihuorong/imgnest-pro), built against a pinned core and adding:

- **SSO**: generic OIDC (Keycloak, Authentik, Casdoor, …), GitHub and Google presets; full PKCE + nonce verification, browser-bound state, per-client rate limits.
- **Licensing**: Ed25519 offline-verified keys with an updates window that decides which builds a key unlocks. Without a valid key every paid feature turns off and everything else behaves exactly like the free edition.

### The SSO email-linking switch

`link_by_email` lets a first sign-in attach to an existing local account with the same **verified** email. Enable it only for providers whose email claims you trust; administrator accounts are never linked automatically.

## Licensing and builds (for operators)

- `licensetool keygen` creates the signing key pair; the private key stays on the issuing machine, and only the public key is injected at build time.
- A build without a public key accepts no license at all — fail-closed by construction.
- Pro configuration is a separate file from the core's (`IMGNEST_PRO_CONFIG`), so Pro secrets never appear in the free edition's configuration surface.
