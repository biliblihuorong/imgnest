package native

import (
	"context"
	"fmt"

	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// Activity reads an account's sign-ins and API usage.
type Activity interface {
	Activity(context.Context, uint64) (service.ActivityView, error)
}

// SystemInfo reports the running version and available updates.
type SystemInfo interface {
	Version(context.Context, bool) (service.SystemVersion, error)
}

// RegisterActivityRoutes binds the account's own activity and the
// administrator's view of any account.
func (h *Handler) RegisterActivityRoutes(ctx context.Context, router gin.IRouter, activity Activity) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("register activity routes: %w", err)
	}
	if activity == nil {
		return fmt.Errorf("register activity routes: missing dependencies")
	}
	router.GET("/api/account/activity", h.authenticate, func(c *gin.Context) {
		view, err := activity.Activity(c.Request.Context(), identity(c).User.ID)
		if err != nil {
			fail(c, err)
			return
		}
		respond(c, 200, view)
	})
	router.GET("/api/admin/users/:id/activity", h.authenticate, adminOnly, func(c *gin.Context) {
		id, ok := adminID(c)
		if !ok {
			return
		}
		view, err := activity.Activity(c.Request.Context(), id)
		if err != nil {
			fail(c, err)
			return
		}
		respond(c, 200, view)
	})
	return nil
}

// RegisterSystemRoutes binds the console's About information. ?refresh=1
// asks for a fresh update check.
func (h *Handler) RegisterSystemRoutes(ctx context.Context, router gin.IRouter, system SystemInfo) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("register system routes: %w", err)
	}
	if system == nil {
		return fmt.Errorf("register system routes: missing dependencies")
	}
	router.GET("/api/admin/system/version", h.authenticate, adminOnly, func(c *gin.Context) {
		refresh := c.Query("refresh") == "1" || c.Query("refresh") == "true"
		view, err := system.Version(c.Request.Context(), refresh)
		if err != nil {
			fail(c, err)
			return
		}
		respond(c, 200, view)
	})
	return nil
}

// adminOnly is middleware that stops authenticated non-administrators.
func adminOnly(c *gin.Context) {
	if !requireAdmin(c) {
		c.Abort()
	}
}
