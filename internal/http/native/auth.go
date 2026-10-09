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
	"time"

	"github.com/biliblihuorong/imgnest/extension"
	"github.com/biliblihuorong/imgnest/internal/http/ratelimit"
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
	VerifyCredentials(context.Context, string, string) (service.VerifiedCredentials, error)
	ChangePassword(context.Context, uint64, string, string) error
	// UpdateDisplayName saves the authenticated user's optional profile name;
	// the identity always comes from the bearer token, never the body.
	UpdateDisplayName(context.Context, uint64, string) (service.UserView, error)
	Site(context.Context) (service.SiteView, error)
}

// TokenService is the shared token business API used by native handlers.
type TokenService interface {
	Issue(context.Context, service.TokenSubject, service.TokenInput) (service.IssuedToken, error)
	Authenticate(context.Context, string) (service.Identity, error)
	List(context.Context, uint64) ([]service.TokenView, error)
	Revoke(context.Context, uint64, uint64) error
}

// Handler binds native endpoint behavior to the shared services.
type Handler struct {
	users   UserService
	captcha CaptchaService
	tokens  TokenService
	now     func() time.Time
	// limits throttles login, registration and uploads per client or user.
	limits *ratelimit.Limiter
	// plugins contribute extra sign-in options to the public site view.
	plugins []extension.Plugin
}

// UsePlugins sets the extensions whose login providers the site view lists.
func (h *Handler) UsePlugins(plugins []extension.Plugin) { h.plugins = plugins }

const identityKey = "imgnest_identity"

// NewHandler creates native handlers with an injectable clock for rate limits.
func NewHandler(ctx context.Context, users UserService, tokens TokenService, now func() time.Time) (*Handler, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("create native handler: %w", err)
	}
	if users == nil || tokens == nil || now == nil {
		return nil, fmt.Errorf("create native handler: missing dependencies")
	}
	return &Handler{users: users, tokens: tokens, now: now, limits: ratelimit.New(now, time.Minute, ratelimit.DefaultCapacity)}, nil
}

// RegisterRoutes binds native routes to the provided router.
func (h *Handler) RegisterRoutes(ctx context.Context, router gin.IRouter) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("register native routes: %w", err)
	}
	router.POST("/api/auth/register", h.rateLimit, h.register)
	router.POST("/api/auth/login", h.rateLimit, h.login)
	router.GET("/api/site", h.site)
	router.GET("/api/auth/captcha", h.publicCaptcha)
	protected := router.Group("/api", h.authenticate)
	protected.GET("/auth/me", h.me)
	protected.PATCH("/auth/profile", h.profile)
	protected.POST("/auth/logout", h.logout)
	protected.PATCH("/auth/password", h.password)
	protected.GET("/tokens", h.listTokens)
	protected.POST("/tokens", h.createToken)
	protected.DELETE("/tokens/:id", h.revokeToken)
	return nil
}

func (h *Handler) rateLimit(c *gin.Context) {
	// ponytail: bounded fixed-window limits suit the monolith; shared limits are needed for multiple instances.
	if !h.limits.Allow(c.FullPath()+":"+ratelimit.ClientKey(c.ClientIP()), 3) {
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
	// The client address is server-derived and must never be part of the
	// decoded request shape.
	var input struct {
		Username     string `json:"username"`
		Email        string `json:"email"`
		Password     string `json:"password"`
		CaptchaToken string `json:"captcha_token,omitempty"`
	}
	if !decode(c, &input) {
		return
	}
	if h.captcha != nil {
		site, err := h.users.Site(c.Request.Context())
		if err != nil {
			fail(c, err)
			return
		}
		if !site.RegisterEnabled {
			fail(c, service.ErrRegistrationDisabled)
			return
		}
	}
	if !h.verifyCaptcha(c, input.CaptchaToken, "register") {
		return
	}
	user, err := h.users.Register(c.Request.Context(), service.RegisterInput{
		Username: input.Username, Email: input.Email, Password: input.Password, IP: c.ClientIP(),
	})
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 201, user)
}
func (h *Handler) login(c *gin.Context) {
	var in struct {
		Email        string `json:"email"`
		Password     string `json:"password"`
		CaptchaToken string `json:"captcha_token,omitempty"`
	}
	if !decode(c, &in) {
		return
	}
	if !h.verifyCaptcha(c, in.CaptchaToken, "login") {
		return
	}
	user, err := h.users.VerifyCredentials(c.Request.Context(), in.Email, in.Password)
	if err != nil {
		fail(c, err)
		return
	}
	issued, err := h.tokens.Issue(c.Request.Context(), user.Subject, service.TokenInput{Name: "web", Kind: "web", Abilities: []string{"*"}})
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, gin.H{"token": issued.Token, "user": user.User, "expires_at": issued.Info.ExpiresAt})
}
func (h *Handler) me(c *gin.Context) { respond(c, 200, identity(c).User) }
func (h *Handler) profile(c *gin.Context) {
	// Only display_name has a request shape; unknown fields are rejected by
	// the decoder, so user_id, role, group, or email cannot be smuggled in.
	var in struct {
		DisplayName *string `json:"display_name"`
	}
	if !decode(c, &in) {
		return
	}
	if in.DisplayName == nil {
		fail(c, service.ErrInvalidInput)
		return
	}
	user, err := h.users.UpdateDisplayName(c.Request.Context(), identity(c).User.ID, *in.DisplayName)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, user)
}
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
	status, response := errorResponse(err)
	c.JSON(status, response)
}

func errorResponse(err error) (int, Response) {
	status, code, message := 500, 50001, "internal error"
	switch {
	case errors.Is(err, service.ErrCaptchaFailed):
		status, code, message = 422, 30010, "complete a new captcha challenge and try again"
	case errors.Is(err, service.ErrCaptchaConflict):
		status, code, message = 409, 30011, "captcha configuration changed; reload and try again"
	case errors.Is(err, service.ErrCaptchaNotReady):
		status, code, message = 409, 30012, "captcha candidate tests and compatibility acknowledgements required"
	case errors.Is(err, service.ErrCaptchaUnavailable):
		status, code, message = 503, 50004, "captcha temporarily unavailable; try again later"
	case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
		status, code, message = 408, 10004, "request timed out or canceled"
	case errors.Is(err, errUploadTooLarge):
		status, code, message = 413, 10002, "upload exceeds size limit"
	case errors.Is(err, errUploadRateLimited):
		status, code, message = 429, 30003, "too many uploads"
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
	case errors.Is(err, service.ErrQuotaExceeded):
		status, code, message = 403, 30004, "capacity exceeded"
	case errors.Is(err, service.ErrPathConflict):
		status, code, message = 409, 30005, "image path conflict"
	case errors.Is(err, service.ErrImageBusy):
		status, code, message = 409, 30006, "image operation in progress"
	case errors.Is(err, service.ErrUnsupportedFormat):
		status, code, message = 415, 30007, "image format not allowed"
	case errors.Is(err, service.ErrGroupHasMembers):
		status, code, message = 409, 30008, "group still has members"
	case errors.Is(err, service.ErrStillReferenced):
		status, code, message = 409, 30009, "resource is still referenced"
	case errors.Is(err, service.ErrStorage):
		status, code, message = 502, 50002, "storage operation failed"
	case errors.Is(err, service.ErrProcessing):
		status, code, message = 422, 50003, "image processing failed"
	}
	return status, Response{Code: code, Message: message, Data: nil}
}
