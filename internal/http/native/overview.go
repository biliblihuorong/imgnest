package native

import (
	"context"
	"fmt"

	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// AdminOverview reads the console's site-wide dashboard.
type AdminOverview interface {
	Overview(context.Context) (service.AdminOverview, error)
}

// RegisterAdminOverviewRoutes binds the administrator dashboard.
func (h *Handler) RegisterAdminOverviewRoutes(ctx context.Context, router gin.IRouter, overview AdminOverview) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("register admin overview routes: %w", err)
	}
	if overview == nil {
		return fmt.Errorf("register admin overview routes: missing dependencies")
	}
	router.GET("/api/admin/overview", h.authenticate, adminOnly, func(c *gin.Context) {
		view, err := overview.Overview(c.Request.Context())
		if err != nil {
			fail(c, err)
			return
		}
		respond(c, 200, view)
	})
	return nil
}
