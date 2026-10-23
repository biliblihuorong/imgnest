package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/biliblihuorong/imgnest/internal/http/reqbody"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// UserPluginSettings serves extensions scoped to the authenticated account.
type UserPluginSettings interface {
	List(context.Context, uint64) ([]service.UserPluginSettingsView, error)
	Save(context.Context, uint64, string, json.RawMessage) (service.UserPluginSettingsView, error)
	Asset(context.Context, uint64, string) ([]byte, string, error)
	PutAsset(context.Context, uint64, string, []byte) (service.UserPluginSettingsView, error)
	DeleteAsset(context.Context, uint64, string) (service.UserPluginSettingsView, error)
}

// RegisterUserPluginSettingsRoutes binds session-only account preferences.
// Owners always come from authentication; assets never have public URLs.
func (h *Handler) RegisterUserPluginSettingsRoutes(ctx context.Context, router gin.IRouter, settings UserPluginSettings) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("register user plugin settings: %w", err)
	}
	if settings == nil {
		return fmt.Errorf("register user plugin settings: missing service")
	}
	slots := make(chan struct{}, 2)
	group := router.Group("/api/user/plugins", h.authenticate)
	group.GET("", func(c *gin.Context) {
		views, err := settings.List(c.Request.Context(), identity(c).User.ID)
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
		if !decode(c, &body) {
			return
		}
		if len(body.Values) == 0 {
			fail(c, service.ErrInvalidInput)
			return
		}
		view, err := settings.Save(c.Request.Context(), identity(c).User.ID, c.Param("name"), body.Values)
		if err != nil {
			fail(c, err)
			return
		}
		respond(c, 200, view)
	})
	group.GET("/:name/asset", func(c *gin.Context) {
		data, mime, err := settings.Asset(c.Request.Context(), identity(c).User.ID, c.Param("name"))
		if err != nil {
			fail(c, err)
			return
		}
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Data(200, mime, data)
	})
	group.PUT("/:name/asset", func(c *gin.Context) {
		owner := identity(c).User.ID
		if !h.limits.Allow("user-plugin-asset:"+strconv.FormatUint(owner, 10), 10) {
			c.AbortWithStatusJSON(429, Response{Code: 30003, Message: "too many requests"})
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			c.AbortWithStatusJSON(429, Response{Code: 30003, Message: "too many requests"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		release := reqbody.Bound(ctx, c.Writer, c.Request, model.MaxPluginUserAssetBytes+(1<<20))
		defer release()
		if err := c.Request.ParseMultipartForm(model.MaxPluginUserAssetBytes); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				fail(c, errUploadTooLarge)
			} else {
				fail(c, service.ErrInvalidInput)
			}
			return
		}
		defer func() {
			if c.Request.MultipartForm != nil {
				_ = c.Request.MultipartForm.RemoveAll()
			}
		}()
		file, _, err := c.Request.FormFile("file")
		if err != nil {
			fail(c, service.ErrInvalidInput)
			return
		}
		defer func() { _ = file.Close() }()
		data, err := io.ReadAll(io.LimitReader(file, model.MaxPluginUserAssetBytes+1))
		if err != nil {
			fail(c, service.ErrInvalidInput)
			return
		}
		if len(data) > model.MaxPluginUserAssetBytes {
			fail(c, service.ErrFileTooLarge)
			return
		}
		view, err := settings.PutAsset(ctx, owner, c.Param("name"), data)
		if err != nil {
			fail(c, err)
			return
		}
		respond(c, 200, view)
	})
	group.DELETE("/:name/asset", func(c *gin.Context) {
		view, err := settings.DeleteAsset(c.Request.Context(), identity(c).User.ID, c.Param("name"))
		if err != nil {
			fail(c, err)
			return
		}
		respond(c, 200, view)
	})
	return nil
}
