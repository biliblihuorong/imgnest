package native

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/biliblihuorong/imgnest/internal/http/ratelimit"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// Variants serves responsive-variant lookups and the historic backfill.
type Variants interface {
	LookupVariants(context.Context, []string) ([]service.VariantLookupItem, error)
	VariantBackfillStatus(context.Context, service.TokenSubject) (service.VariantBackfillView, error)
	StartVariantBackfill(context.Context, service.TokenSubject, bool) (service.VariantBackfillView, error)
	PauseVariantBackfill(context.Context, service.TokenSubject) (service.VariantBackfillView, error)
}

// VariantOptions configures the public lookup endpoint.
type VariantOptions struct {
	// CORSOrigins may read lookups from browsers; "*" allows any origin.
	CORSOrigins []string
	// PerMinute bounds lookups per client address; zero uses 60.
	PerMinute int
	// CacheMaxAge is the browser lifetime of a successful lookup.
	CacheMaxAge time.Duration
	// CDNMaxAge is how long a CDN or other shared cache may keep a
	// successful GET lookup (s-maxage); zero leaves it at CacheMaxAge.
	CDNMaxAge time.Duration
	Now       func() time.Time
}

// RegisterVariantRoutes binds the public lookup and the admin backfill.
func (h *Handler) RegisterVariantRoutes(ctx context.Context, router gin.IRouter, variants Variants, opts VariantOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("register variant routes: %w", err)
	}
	if variants == nil || opts.PerMinute < 0 || opts.CacheMaxAge < 0 || opts.CDNMaxAge < 0 {
		return service.ErrInvalidInput
	}
	if opts.PerMinute == 0 {
		opts.PerMinute = 60
	}
	if opts.CacheMaxAge == 0 {
		opts.CacheMaxAge = 5 * time.Minute
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	handler := &variantHandler{variants: variants, options: opts, limits: ratelimit.New(opts.Now, time.Minute, ratelimit.DefaultCapacity), origins: map[string]bool{}}
	for _, origin := range opts.CORSOrigins {
		handler.origins[strings.TrimRight(strings.TrimSpace(origin), "/")] = true
	}
	handler.cacheControl = fmt.Sprintf("public, max-age=%d", int(opts.CacheMaxAge.Seconds()))
	if opts.CDNMaxAge > 0 {
		handler.cacheControl += fmt.Sprintf(", s-maxage=%d, stale-while-revalidate=60", int(opts.CDNMaxAge.Seconds()))
	}
	router.GET("/api/variants", handler.cors, handler.lookupQuery)
	router.POST("/api/variants", handler.cors, handler.lookupBody)
	router.OPTIONS("/api/variants", handler.cors, handler.preflight)
	admin := router.Group("/api/admin/variants", h.authenticate)
	admin.GET("/backfill", handler.backfillStatus)
	admin.POST("/backfill", handler.backfillControl)
	return nil
}

type variantHandler struct {
	variants     Variants
	options      VariantOptions
	limits       *ratelimit.Limiter
	origins      map[string]bool
	cacheControl string
}

// cors answers configured origins only. Only a list of named origins makes
// the response depend on Origin, so only then does it say Vary: Origin; with
// no origins or "*" every visitor gets the same bytes and a CDN keeps one
// copy per URL instead of one per referring site.
func (h *variantHandler) cors(c *gin.Context) {
	if len(h.origins) > 0 && !h.origins["*"] {
		c.Header("Vary", "Origin")
	}
	origin := c.GetHeader("Origin")
	if origin == "" || len(h.origins) == 0 {
		return
	}
	if h.origins["*"] {
		c.Header("Access-Control-Allow-Origin", "*")
	} else if h.origins[origin] {
		c.Header("Access-Control-Allow-Origin", origin)
	}
}

func (h *variantHandler) preflight(c *gin.Context) {
	if c.Writer.Header().Get("Access-Control-Allow-Origin") != "" {
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		c.Header("Access-Control-Max-Age", "86400")
	}
	c.Status(http.StatusNoContent)
}

func (h *variantHandler) lookupQuery(c *gin.Context) {
	h.lookup(c, c.QueryArray("url"))
}

func (h *variantHandler) lookupBody(c *gin.Context) {
	var input struct {
		URLs []string `json:"urls"`
	}
	if !decode(c, &input) {
		return
	}
	h.lookup(c, input.URLs)
}

func (h *variantHandler) lookup(c *gin.Context, links []string) {
	if !h.limits.Allow("variants:"+c.ClientIP(), h.options.PerMinute) {
		c.JSON(http.StatusTooManyRequests, Response{Code: 30003, Message: "too many requests", Data: nil})
		return
	}
	if len(links) == 0 || len(links) > service.MaxVariantLookup {
		fail(c, service.ErrInvalidInput)
		return
	}
	items, err := h.variants.LookupVariants(c.Request.Context(), links)
	if err != nil {
		fail(c, err)
		return
	}
	body, err := json.Marshal(Response{Code: 0, Message: "ok", Data: gin.H{"items": items}})
	if err != nil {
		fail(c, err)
		return
	}
	// The ETag lets a browser or CDN revalidate with a 304 instead of
	// downloading the same answer again once its copy goes stale.
	digest := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(digest[:16]) + `"`
	c.Header("Cache-Control", h.cacheControl)
	c.Header("ETag", etag)
	if c.Request.Method == http.MethodGet && etagMatches(c.GetHeader("If-None-Match"), etag) {
		c.Status(http.StatusNotModified)
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// etagMatches reports whether an If-None-Match header names etag (weak
// comparison, as RFC 9110 requires for If-None-Match).
func etagMatches(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "W/")
		if candidate == "*" || candidate == etag {
			return true
		}
	}
	return false
}

func requireAdmin(c *gin.Context) bool {
	if identity(c).User.Role != model.UserRoleAdmin {
		fail(c, service.ErrForbidden)
		return false
	}
	return true
}

func (h *variantHandler) backfillStatus(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	state, err := h.variants.VariantBackfillStatus(c.Request.Context(), identity(c).Subject)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, http.StatusOK, state)
}

func (h *variantHandler) backfillControl(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	var input struct {
		Action  string `json:"action"`
		Restart bool   `json:"restart"`
	}
	if !decode(c, &input) {
		return
	}
	var (
		state service.VariantBackfillView
		err   error
	)
	switch input.Action {
	case "start":
		state, err = h.variants.StartVariantBackfill(c.Request.Context(), identity(c).Subject, input.Restart)
	case "pause":
		if input.Restart {
			fail(c, service.ErrInvalidInput)
			return
		}
		state, err = h.variants.PauseVariantBackfill(c.Request.Context(), identity(c).Subject)
	default:
		fail(c, service.ErrInvalidInput)
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, http.StatusOK, state)
}
