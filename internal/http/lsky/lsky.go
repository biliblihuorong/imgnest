// Package lsky serves the Lsky Pro v1 compatible API. It only maps protocol:
// every business rule runs through the shared services, and responses follow
// the Lsky envelope {"status","message","data"} field for field.
package lsky

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/biliblihuorong/imgnest/internal/http/ratelimit"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// Options bound v1 multipart uploads, mirroring the native image options.
type Options struct {
	MaxRequestBytes int64
	MaxConcurrent   int
	Timeout         time.Duration
	// TrustedProxies lists the addresses or CIDRs whose X-Forwarded-Proto is
	// honoured when building pagination links; others are ignored.
	TrustedProxies []string
}

// Dependencies bind the v1 handlers to the shared services.
type Dependencies struct {
	Users  *service.UserService
	Tokens *service.TokenService
	Images *service.ImageService
	Albums *service.AlbumService
	Lsky   *service.LskyService
	Now    func() time.Time
	// Options may be zero; defaults match the native upload handler.
	Options Options
}

// Handler serves the v1 routes on top of the shared services.
type Handler struct {
	users   *service.UserService
	tokens  *service.TokenService
	images  *service.ImageService
	albums  *service.AlbumService
	lsky    *service.LskyService
	now     func() time.Time
	options Options
	slots   chan struct{}
	limits  *ratelimit.Limiter
	// logins counts failed password exchanges per account, because the v1
	// token route cannot carry the native captcha.
	logins *ratelimit.Limiter
	// proxies are the parsed Options.TrustedProxies.
	proxies []netip.Prefix
}

const identityKey = "lsky_identity"

// NewHandler constructs the v1 handler with an injectable clock.
func NewHandler(ctx context.Context, deps Dependencies) (*Handler, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("create lsky handler: %w", err)
	}
	if deps.Users == nil || deps.Tokens == nil || deps.Images == nil || deps.Albums == nil || deps.Lsky == nil || deps.Now == nil {
		return nil, fmt.Errorf("create lsky handler: missing dependencies")
	}
	if deps.Options.MaxRequestBytes <= 0 {
		deps.Options.MaxRequestBytes = 64 << 20
	}
	if deps.Options.MaxConcurrent <= 0 {
		deps.Options.MaxConcurrent = 2
	}
	if deps.Options.Timeout <= 0 {
		deps.Options.Timeout = 5 * time.Minute
	}
	proxies, err := parseProxies(deps.Options.TrustedProxies)
	if err != nil {
		return nil, fmt.Errorf("create lsky handler: %w", err)
	}
	return &Handler{
		proxies: proxies,
		users:   deps.Users, tokens: deps.Tokens, images: deps.Images,
		albums: deps.Albums, lsky: deps.Lsky, now: deps.Now, options: deps.Options,
		slots:  make(chan struct{}, deps.Options.MaxConcurrent),
		limits: ratelimit.New(deps.Now, time.Minute, ratelimit.DefaultCapacity),
		logins: ratelimit.New(deps.Now, loginFailureWindow, ratelimit.DefaultCapacity),
	}, nil
}

// RegisterRoutes binds the v1 API to the router. Every route rejects traffic
// while the API switch is disabled.
func (h *Handler) RegisterRoutes(ctx context.Context, router gin.IRouter) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("register lsky routes: %w", err)
	}
	v1 := router.Group("/api/v1", h.apiEnabled)
	v1.POST("/tokens", h.tokenThrottle, h.createToken)
	v1.DELETE("/tokens", h.authenticate, h.revokeTokens)
	v1.GET("/strategies", h.optionalIdentity, h.strategies)
	v1.POST("/upload", h.optionalIdentity, h.upload)
	v1.GET("/images", h.authenticate, h.listImages)
	v1.DELETE("/images/:key", h.authenticate, h.deleteImage)
	v1.GET("/albums", h.authenticate, h.listAlbums)
	v1.DELETE("/albums/:id", h.authenticate, h.deleteAlbum)
	v1.GET("/profile", h.authenticate, h.profile)
	return nil
}

// apiEnabled aborts with 403 while the administrator disabled the API.
func (h *Handler) apiEnabled(c *gin.Context) {
	enabled, err := h.lsky.APIEnabled(c.Request.Context())
	if err != nil {
		h.serverError(c)
		c.Abort()
		return
	}
	if !enabled {
		c.JSON(403, failure("管理员未启用 API"))
		c.Abort()
	}
}

// authenticate requires a valid bearer token.
func (h *Handler) authenticate(c *gin.Context) {
	id, ok := h.verifyAuthorization(c)
	if !ok {
		c.JSON(401, failure("Unauthenticated."))
		c.Abort()
		return
	}
	c.Set(identityKey, id)
}

// optionalIdentity authenticates a present Authorization header (an invalid
// one is always rejected, never downgraded to guest) and marks anonymous
// requests as guests.
func (h *Handler) optionalIdentity(c *gin.Context) {
	if header := c.GetHeader("Authorization"); header == "" {
		c.Set(identityKey, (*service.Identity)(nil))
		return
	}
	id, ok := h.verifyAuthorization(c)
	if !ok {
		c.JSON(401, failure("Unauthenticated."))
		c.Abort()
		return
	}
	c.Set(identityKey, id)
}

func (h *Handler) verifyAuthorization(c *gin.Context) (*service.Identity, bool) {
	parts := splitBearer(c.GetHeader("Authorization"))
	if parts == "" {
		return nil, false
	}
	id, err := h.tokens.Authenticate(c.Request.Context(), parts)
	if err != nil {
		return nil, false
	}
	return &id, true
}

func splitBearer(header string) string {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

// currentIdentity returns the authenticated identity or nil for guests.
func currentIdentity(c *gin.Context) *service.Identity {
	id, _ := c.MustGet(identityKey).(*service.Identity)
	return id
}

// Failed v1 password exchanges per account before the account is paused.
const (
	loginFailureLimit  = 10
	loginFailureWindow = 15 * time.Minute
)

// tokenThrottle caps token issuance at three attempts per IP per minute.
func (h *Handler) tokenThrottle(c *gin.Context) {
	if !h.allow("v1tokens:"+ratelimit.ClientKey(c.ClientIP()), 3) {
		c.JSON(429, failure("Too Many Attempts."))
		c.Abort()
	}
}

// allow implements the native fixed-window limiter semantics.
func (h *Handler) allow(key string, limit int) bool {
	return h.limits.Allow(key, limit)
}

func parseProxies(values []string) ([]netip.Prefix, error) {
	proxies := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		if addr, err := netip.ParseAddr(value); err == nil {
			addr = addr.Unmap()
			proxies = append(proxies, netip.PrefixFrom(addr, addr.BitLen()))
			continue
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return nil, errors.New("invalid trusted proxy")
		}
		proxies = append(proxies, prefix.Masked())
	}
	return proxies, nil
}

// forwardedProto returns X-Forwarded-Proto only when the direct peer is a
// configured trusted proxy, matching the router's client-IP policy.
func (h *Handler) forwardedProto(c *gin.Context) string {
	peer, err := netip.ParseAddr(c.RemoteIP())
	if err != nil {
		return ""
	}
	peer = peer.Unmap()
	for _, prefix := range h.proxies {
		if prefix.Contains(peer) {
			return c.GetHeader("X-Forwarded-Proto")
		}
	}
	return ""
}

// envelope is the v1 response shell. Empty data is always an object.
type envelope struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func failure(message string) envelope {
	return envelope{Status: false, Message: message, Data: gin.H{}}
}

func success(message string, data any) envelope {
	if data == nil {
		data = gin.H{}
	}
	return envelope{Status: true, Message: message, Data: data}
}

func (h *Handler) serverError(c *gin.Context) {
	c.JSON(500, failure("服务器内部错误"))
}

func isCanceled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
