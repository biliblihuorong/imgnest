// Package http builds the native HTTP API and its request middleware.
package http

import (
	"context"
	"fmt"
	"log/slog"
	stdhttp "net/http"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/http/native"
	"github.com/gin-gonic/gin"
)

// Dependencies are the application capabilities consumed by the HTTP layer.
type Dependencies struct {
	Users  native.UserService
	Tokens native.TokenService
	Logger *slog.Logger
	Server config.Server
	Now    func() time.Time
	Health func(context.Context) error
}

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
	router.GET("/healthz", func(c *gin.Context) {
		if err := deps.Health(c.Request.Context()); err != nil {
			c.JSON(503, native.Response{Code: 50001, Message: "unavailable", Data: nil})
			return
		}
		c.JSON(200, native.Response{Code: 0, Message: "ok", Data: gin.H{"status": "ok"}})
	})
	router.NoRoute(func(c *gin.Context) { c.JSON(404, native.Response{Code: 10001, Message: "not found", Data: nil}) })
	return router, nil
}
