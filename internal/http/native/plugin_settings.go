package native

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// PluginSettings is the console API for plugin settings cards.
type PluginSettings interface {
	List(context.Context) ([]service.PluginSettingsView, error)
	Save(context.Context, string, json.RawMessage) (service.PluginSettingsView, error)
}

// RegisterPluginSettingsRoutes binds the administrator-only plugin settings
// API. Without plugins the list is empty and the console shows nothing.
func (h *Handler) RegisterPluginSettingsRoutes(ctx context.Context, router gin.IRouter, settings PluginSettings) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("register plugin settings routes: %w", err)
	}
	if settings == nil {
		return fmt.Errorf("register plugin settings routes: missing service")
	}
	admin := &adminHandler{auth: h}
	group := router.Group("/api/admin/plugins", h.authenticate, admin.requireAdmin)
	group.GET("", func(c *gin.Context) {
		views, err := settings.List(c.Request.Context())
		if err != nil {
			fail(c, err)
			return
		}
		respond(c, 200, views)
	})
	group.PUT("/:name/settings", func(c *gin.Context) {
		var body struct {
			Values json.RawMessage `json:"values"`
		}
		if !decode(c, &body) || len(body.Values) == 0 {
			if !c.Writer.Written() {
				fail(c, service.ErrInvalidInput)
			}
			return
		}
		view, err := settings.Save(c.Request.Context(), c.Param("name"), body.Values)
		if err != nil {
			fail(c, err)
			return
		}
		respond(c, 200, view)
	})
	return nil
}
