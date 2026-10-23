package native

import (
	"context"
	"fmt"

	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// APISwitch reports whether the administrator allows token API access; the
// same "api_enabled" setting governs the Lsky v1 routes.
type APISwitch interface {
	APIEnabled(context.Context) (bool, error)
}

// errAPIDisabled is the 403 a v2 client gets while the API is switched off.
var errAPIDisabled = &service.ReasonError{Reason: "api_disabled", Err: service.ErrForbidden}

// RegisterV2Routes mounts ImgNest's own versioned API for scripts and
// upload clients. It serves the same handlers and services as the console
// routes under /api (so behavior never drifts between them) with the native
// {code, message, data} envelope, and is frozen as a contract: fields are
// only ever added. The Lsky-compatible /api/v1 stays untouched alongside it.
// Call it after the image and album routes; a nil switch keeps v2 enabled.
func (h *Handler) RegisterV2Routes(ctx context.Context, router gin.IRouter, api APISwitch) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("register v2 routes: %w", err)
	}
	h.api = api
	v2 := router.Group("/api/v2", func(c *gin.Context) {
		if api == nil {
			return
		}
		enabled, err := api.APIEnabled(c.Request.Context())
		if err != nil {
			fail(c, err)
			c.Abort()
			return
		}
		if !enabled {
			fail(c, errAPIDisabled)
			c.Abort()
		}
	}, h.authenticate)
	v2.GET("/profile", h.me)
	if image := h.v2Images; image != nil {
		v2.GET("/policies", image.listPolicies)
		v2.POST("/upload", image.upload)
		v2.GET("/images", image.list)
		v2.GET("/images/:id", image.get)
		v2.PATCH("/images/:id", image.permission)
		v2.DELETE("/images/:id", image.remove)
		v2.POST("/images/batch", image.batch)
		v2.GET("/albums/:id/images", image.listAlbumSearch)
		v2.GET("/trash", image.listTrash)
		v2.POST("/trash/restore", image.restore)
	}
	if album := h.v2Albums; album != nil {
		v2.GET("/albums", album.list)
		v2.POST("/albums", album.create)
		v2.PATCH("/albums/:id", album.update)
		v2.DELETE("/albums/:id", album.remove)
	}
	return nil
}
