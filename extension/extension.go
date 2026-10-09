// Package extension is the stable surface that editions built outside this
// repository (such as ImgNest Pro) use to add features to the server. Packages
// under internal/ stay private; everything an extension may rely on is here.
package extension

import (
	"context"

	"github.com/gin-gonic/gin"
)

// LoginProvider is an extra sign-in option the login page offers next to the
// password form, such as "Sign in with GitHub".
type LoginProvider struct {
	// ID is a short stable key, unique among the providers of one plugin.
	ID string `json:"id"`
	// Name is the button label shown to visitors.
	Name string `json:"name"`
	// StartURL is the same-origin path the button navigates to; the plugin's
	// own route there starts the sign-in flow.
	StartURL string `json:"start_url"`
}

// Plugin is one server extension. Plugins are passed to app.Execute and are
// mounted when the serve command builds its router.
type Plugin interface {
	// Name is a lowercase identifier ([a-z0-9-], 1 to 32 characters); the
	// plugin's routes live under /api/ext/{Name}.
	Name() string
	// Mount registers the plugin's routes on its own /api/ext/{Name} group, so
	// a plugin can never shadow a core route. host finishes sign-ins.
	Mount(ctx context.Context, router gin.IRouter, host Host) error
	// LoginProviders lists the sign-in options to show on the login page. It
	// is called for every public site read and must be cheap.
	LoginProviders(ctx context.Context) []LoginProvider
}

// ExternalIdentity is a user the plugin has authenticated with an external
// provider, for example from a verified OIDC ID token.
type ExternalIdentity struct {
	// Provider is the plugin's provider ID ("github"); the host namespaces it
	// with the plugin name before storing it.
	Provider string
	// Subject is the provider's stable, never-reassigned user ID. Never use an
	// email address or a renameable login here.
	Subject string
	Email   string
	// EmailVerified is true only when the provider asserts the address.
	EmailVerified bool
	// Username seeds the username of an account created on first sign-in.
	Username string
	// LinkByEmail lets a first-time subject attach to an existing
	// non-administrator account with the same verified email. Enable it only
	// for providers whose email claims you trust.
	LinkByEmail bool
}

// Host is what ImgNest offers a mounted plugin.
type Host interface {
	// CompleteSignIn signs the identity in (linking it, or creating an
	// account when registration is open) and redirects the browser back to
	// the web app, which finishes the login. It always writes the response.
	CompleteSignIn(c *gin.Context, identity ExternalIdentity)
	// FailSignIn redirects the browser to the login page with a generic
	// error, for flows the plugin itself rejected (bad state, denied consent).
	FailSignIn(c *gin.Context)
}
