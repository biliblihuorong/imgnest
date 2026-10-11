// Package http builds the native HTTP API and its request middleware.
package http

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	stdhttp "net/http"
	"regexp"
	"strings"
	"time"

	"github.com/biliblihuorong/imgnest/extension"
	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/http/lsky"
	"github.com/biliblihuorong/imgnest/internal/http/native"
	"github.com/gin-gonic/gin"
)

// Dependencies are the application capabilities consumed by the HTTP layer.
type Dependencies struct {
	Captcha      native.CaptchaService
	Users        native.UserService
	Tokens       native.TokenService
	Logger       *slog.Logger
	Server       config.Server
	Now          func() time.Time
	Health       func(context.Context) error
	Images       native.ImageService
	ImageOptions native.ImageOptions
	// Albums serves the native album management routes when configured.
	Albums native.Albums
	// RandomLinks serves the anonymous /random redirect and its per-album
	// management routes when configured.
	RandomLinks native.RandomLinks
	// Lsky serves the Lsky-compatible /api/v1 routes when configured.
	Lsky *lsky.Handler
	// Admin serves the /api/admin management routes when configured.
	Admin native.AdminService
	// AdminImages is the site-wide image portion of the management console.
	AdminImages native.AdminImages
	// Web is the built single-page app rooted at its dist directory. When nil
	// the router keeps returning JSON 404 for unmatched paths.
	Web fs.FS
	// Plugins are out-of-tree extensions, each mounted under /api/ext/{name}.
	Plugins []extension.Plugin
	// ExternalSignIn signs in identities that plugins verified; without it
	// plugin sign-ins end on the login page with an "unavailable" error.
	ExternalSignIn native.ExternalSignIn
}

var pluginName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// ImageOptions configures bounded native image uploads.
type ImageOptions = native.ImageOptions

// NewRouter constructs the native API without depending on a concrete repository.
func NewRouter(ctx context.Context, deps Dependencies) (stdhttp.Handler, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("create router: %w", err)
	}
	if deps.Users == nil || deps.Tokens == nil || deps.Logger == nil || deps.Health == nil {
		return nil, fmt.Errorf("create router: missing dependencies")
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	router := gin.New()
	if err := router.SetTrustedProxies(deps.Server.TrustedProxies); err != nil {
		return nil, fmt.Errorf("configure trusted proxies: invalid address")
	}
	router.Use(func(c *gin.Context) {
		started := time.Now()
		c.Header("Cache-Control", "no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		}
		deps.Logger.InfoContext(c.Request.Context(), "http request", "method", c.Request.Method, "route", path, "status", c.Writer.Status(), "duration_ms", time.Since(started).Milliseconds())
	})
	router.Use(func(c *gin.Context) {
		defer func() {
			if recover() != nil {
				deps.Logger.ErrorContext(c.Request.Context(), "request panic", "route", c.FullPath())
				// The Lsky-compatible routes answer with their own envelope
				// so a panic never breaks a v1 client's error parsing.
				if strings.HasPrefix(c.FullPath(), "/api/v1") {
					c.AbortWithStatusJSON(500, gin.H{"status": false, "message": "internal error", "data": gin.H{}})
					return
				}
				c.AbortWithStatusJSON(500, native.Response{Code: 50001, Message: "internal error", Data: nil})
			}
		}()
		c.Next()
	})
	handler, err := native.NewHandler(ctx, deps.Users, deps.Tokens, deps.Now)
	if err != nil {
		return nil, fmt.Errorf("create native handler: %w", err)
	}
	if err := handler.RegisterRoutes(ctx, router); err != nil {
		return nil, fmt.Errorf("register native routes: %w", err)
	}
	handler.UsePlugins(deps.Plugins, deps.ExternalSignIn)
	if err := mountPlugins(ctx, router, deps.Plugins, handler); err != nil {
		return nil, err
	}
	if deps.Captcha != nil {
		if err := handler.RegisterCaptchaRoutes(ctx, router, deps.Captcha); err != nil {
			return nil, fmt.Errorf("register captcha routes: %w", err)
		}
	}
	if deps.Images != nil {
		if err := handler.RegisterImageRoutes(ctx, router, deps.Images, deps.ImageOptions); err != nil {
			return nil, fmt.Errorf("register image routes: %w", err)
		}
	}
	if deps.Albums != nil {
		if err := handler.RegisterAlbumRoutes(ctx, router, deps.Albums); err != nil {
			return nil, fmt.Errorf("register album routes: %w", err)
		}
	}
	if deps.RandomLinks != nil {
		if err := handler.RegisterRandomLinkRoutes(ctx, router, deps.RandomLinks); err != nil {
			return nil, fmt.Errorf("register random link routes: %w", err)
		}
	}
	if deps.Admin != nil {
		if err := handler.RegisterAdminRoutes(ctx, router, deps.Admin, deps.AdminImages); err != nil {
			return nil, fmt.Errorf("register admin routes: %w", err)
		}
	}
	if deps.Lsky != nil {
		if err := deps.Lsky.RegisterRoutes(ctx, router); err != nil {
			return nil, fmt.Errorf("register lsky routes: %w", err)
		}
	}
	router.GET("/healthz", func(c *gin.Context) {
		if err := deps.Health(c.Request.Context()); err != nil {
			c.JSON(503, native.Response{Code: 50001, Message: "unavailable", Data: nil})
			return
		}
		c.JSON(200, native.Response{Code: 0, Message: "ok", Data: gin.H{"status": "ok"}})
	})
	if deps.Web != nil {
		spa := &spaHandler{files: deps.Web}
		router.NoRoute(spa.handle)
	} else {
		router.NoRoute(func(c *gin.Context) { c.JSON(404, native.Response{Code: 10001, Message: "not found", Data: nil}) })
	}
	return router, nil
}

// mountPlugins gives each plugin its own route group so no plugin can shadow a
// core route or another plugin's routes.
func mountPlugins(ctx context.Context, router gin.IRouter, plugins []extension.Plugin, handler *native.Handler) error {
	seen := make(map[string]bool, len(plugins))
	for _, plugin := range plugins {
		if plugin == nil {
			return fmt.Errorf("mount plugin: nil plugin")
		}
		name := plugin.Name()
		if !pluginName.MatchString(name) {
			return fmt.Errorf("mount plugin %q: invalid name", name)
		}
		if seen[name] {
			return fmt.Errorf("mount plugin %q: duplicate name", name)
		}
		seen[name] = true
		if err := plugin.Mount(ctx, router.Group("/api/ext/"+name), handler.Host(name)); err != nil {
			return fmt.Errorf("mount plugin %q: %w", name, err)
		}
	}
	return nil
}
