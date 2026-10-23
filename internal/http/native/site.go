package native

import (
	"github.com/biliblihuorong/imgnest/extension"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// siteResponse adds extension sign-in options to the service's site view;
// login_providers is always an array so the login page can range over it.
type siteResponse struct {
	service.SiteView
	BillingEnabled bool                      `json:"billing_enabled,omitempty"`
	LoginProviders []extension.LoginProvider `json:"login_providers"`
}

// site serves the public site descriptor and never exposes other settings.
func (h *Handler) site(c *gin.Context) {
	view, err := h.users.Site(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	providers := []extension.LoginProvider{}
	billingEnabled := false
	for _, plugin := range h.plugins {
		providers = append(providers, plugin.LoginProviders(c.Request.Context())...)
		if p, ok := plugin.(extension.BillingProvider); ok {
			store, enabled := p.BillingService(c.Request.Context())
			billingEnabled = billingEnabled || enabled && store != nil
		}
	}
	respond(c, 200, siteResponse{SiteView: view, LoginProviders: providers, BillingEnabled: billingEnabled})
}
