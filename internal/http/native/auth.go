// Package native implements ImgNest's native API contract.
package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// Response is the native API's common response envelope.
type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// UserService is the user business API used by native authentication handlers.
type UserService interface {
	Register(context.Context, service.RegisterInput) (service.UserView, error)
	VerifyCredentials(context.Context, string, string) (service.UserView, error)
	ChangePassword(context.Context, uint64, string, string) error
}

// TokenService is the shared token business API used by native handlers.
type TokenService interface {
	Issue(context.Context, uint64, service.TokenInput) (service.IssuedToken, error)
	Authenticate(context.Context, string) (service.Identity, error)
	List(context.Context, uint64) ([]service.TokenView, error)
	Revoke(context.Context, uint64, uint64) error
}

// Handler binds native endpoint behavior to the shared services.
type Handler struct {
	users  UserService
	tokens TokenService
	now    func() time.Time
	mu     sync.Mutex
	limits map[string]window
}
type window struct {
	until time.Time
	count int
}

const identityKey = "imgnest_identity"

// NewHandler creates native handlers with an injectable clock for rate limits.
func NewHandler(ctx context.Context, users UserService, tokens TokenService, now func() time.Time) (*Handler, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("create native handler: %w", err)
	}
	if users == nil || tokens == nil || now == nil {
		return nil, fmt.Errorf("create native handler: missing dependencies")
	}
	return &Handler{users: users, tokens: tokens, now: now, limits: make(map[string]window)}, nil
}

// RegisterRoutes binds native routes to the provided router.
func (h *Handler) RegisterRoutes(ctx context.Context, router gin.IRouter) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("register native routes: %w", err)
	}
	router.POST("/api/auth/register", h.rateLimit, h.register)
	router.POST("/api/auth/login", h.rateLimit, h.login)
	protected := router.Group("/api", h.authenticate)
	protected.GET("/auth/me", h.me)
	protected.POST("/auth/logout", h.logout)
	protected.PATCH("/auth/password", h.password)
	protected.GET("/tokens", h.listTokens)
	protected.POST("/tokens", h.createToken)
	protected.DELETE("/tokens/:id", h.revokeToken)
	return nil
}

func (h *Handler) rateLimit(c *gin.Context) {
	key := c.FullPath() + ":" + c.ClientIP()
	now := h.now()
	h.mu.Lock()
	entry, exists := h.limits[key]
	if !exists || !now.Before(entry.until) {
		// ponytail: bounded fixed-window limits suit the monolith; shared limits are needed for multiple instances.
		for other, current := range h.limits {
			if !now.Before(current.until) {
				delete(h.limits, other)
			}
		}
		if len(h.limits) >= 4096 {
			h.mu.Unlock()
			c.AbortWithStatusJSON(429, Response{Code: 30003, Message: "too many requests", Data: nil})
			return
		}
		entry = window{until: now.Add(time.Minute)}
	}
	entry.count++
	h.limits[key] = entry
	h.mu.Unlock()
	if entry.count > 3 {
		c.AbortWithStatusJSON(429, Response{Code: 30003, Message: "too many requests", Data: nil})
	}
}

func (h *Handler) authenticate(c *gin.Context) {
	parts := strings.Fields(c.GetHeader("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		fail(c, service.ErrUnauthenticated)
		c.Abort()
		return
	}
	id, err := h.tokens.Authenticate(c.Request.Context(), parts[1])
	if err != nil {
		fail(c, err)
		c.Abort()
		return
	}
	c.Set(identityKey, id)
}
func identity(c *gin.Context) service.Identity { return c.MustGet(identityKey).(service.Identity) }
func (h *Handler) register(c *gin.Context) {
	var in service.RegisterInput
	if !decode(c, &in) {
		return
	}
	user, err := h.users.Register(c.Request.Context(), in)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 201, user)
}
func (h *Handler) login(c *gin.Context) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(c, &in) {
		return
	}
	user, err := h.users.VerifyCredentials(c.Request.Context(), in.Email, in.Password)
	if err != nil {
		fail(c, err)
		return
	}
	issued, err := h.tokens.Issue(c.Request.Context(), user.ID, service.TokenInput{Name: "web", Kind: "web", Abilities: []string{"*"}})
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, gin.H{"token": issued.Token, "user": user, "expires_at": issued.Info.ExpiresAt})
}
func (h *Handler) me(c *gin.Context) { respond(c, 200, identity(c).User) }
func (h *Handler) logout(c *gin.Context) {
	id := identity(c)
	if err := h.tokens.Revoke(c.Request.Context(), id.User.ID, id.TokenID); err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, nil)
}
func (h *Handler) password(c *gin.Context) {
	var in struct {
		Current string `json:"current_password"`
		Next    string `json:"new_password"`
	}
	if !decode(c, &in) {
		return
	}
	if err := h.users.ChangePassword(c.Request.Context(), identity(c).User.ID, in.Current, in.Next); err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, nil)
}
func decode(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		fail(c, service.ErrInvalidInput)
		return false
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		fail(c, service.ErrInvalidInput)
		return false
	}
	return true
}
func respond(c *gin.Context, status int, data any) {
	c.JSON(status, Response{Code: 0, Message: "ok", Data: data})
}
func fail(c *gin.Context, err error) {
	status, code, message := 500, 50001, "internal error"
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		status, code, message = 400, 10001, "invalid request"
	case errors.Is(err, service.ErrUnauthenticated):
		status, code, message = 401, 20001, "unauthenticated"
	case errors.Is(err, service.ErrInvalidCredentials):
		status, code, message = 401, 20002, "invalid credentials"
	case errors.Is(err, service.ErrForbidden):
		status, code, message = 403, 20003, "forbidden"
	case errors.Is(err, service.ErrRegistrationDisabled):
		status, code, message = 403, 30001, "registration disabled"
	case errors.Is(err, service.ErrUserExists):
		status, code, message = 409, 30002, "user already exists"
	case errors.Is(err, service.ErrNotFound):
		status, code, message = 404, 10001, "not found"
	}
	c.JSON(status, Response{Code: code, Message: message, Data: nil})
}
