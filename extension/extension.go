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
	// a plugin can never shadow a core route.
	Mount(ctx context.Context, router gin.IRouter) error
	// LoginProviders lists the sign-in options to show on the login page. It
	// is called for every public site read and must be cheap.
	LoginProviders(ctx context.Context) []LoginProvider
}
