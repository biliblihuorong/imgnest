package native

import (
	"context"
	"net/http"
	"time"

	"github.com/biliblihuorong/imgnest/extension"
	"github.com/biliblihuorong/imgnest/internal/http/ratelimit"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// Random-route limits are far looser than the login limiter's: image embeds
// legitimately repeat, and proxies such as README image caches share addresses.
const (
	randomRequestsPerMinute = 600
	randomLimiterCapacity   = 16384
	randomPublicIDLength    = 10
	randomTokenLength       = 24
)

// RandomLinks is the random-image-link business API consumed by native routes.
type RandomLinks interface {
	Get(ctx context.Context, ownerID, albumID uint64) (*service.RandomLinkView, error)
	Put(ctx context.Context, ownerID, albumID uint64, enabled bool) (service.RandomLinkView, error)
	Reset(ctx context.Context, ownerID, albumID uint64) (service.RandomLinkView, error)
	Delete(ctx context.Context, ownerID, albumID uint64) error
	Pick(ctx context.Context, uid, token string, original bool) (string, error)
}

// RegisterRandomLinkRoutes binds the anonymous redirect route and the
// owner-only link management routes.
func (h *Handler) RegisterRandomLinkRoutes(ctx context.Context, router gin.IRouter, links RandomLinks) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if links == nil {
		return service.ErrInvalidInput
	}
	random := &randomHandler{
		links: links,
		// A private table keeps a flood here from filling the login limiter's;
		// the evicting limiter keeps a flood of distinct clients from locking
		// everyone else out, and ClientKey folds IPv6 into /64 blocks.
		limiter: ratelimit.New(h.now, time.Minute, randomLimiterCapacity),
	}
	guard := h.guardAccess(extension.AccessRandom)
	router.GET("/random/:uid/:token", random.rateLimit, guard, random.redirect)
	router.HEAD("/random/:uid/:token", random.rateLimit, guard, random.redirect)
	protected := router.Group("/api", h.authenticate)
	protected.GET("/albums/:id/random-link", random.get)
	protected.PUT("/albums/:id/random-link", random.put)
	protected.POST("/albums/:id/random-link/reset", random.reset)
	protected.DELETE("/albums/:id/random-link", random.remove)
	return nil
}

type randomHandler struct {
	links   RandomLinks
	limiter *ratelimit.Limiter
}

func (h *randomHandler) rateLimit(c *gin.Context) {
	if !h.limiter.Allow(ratelimit.ClientKey(c.ClientIP()), randomRequestsPerMinute) {
		c.AbortWithStatusJSON(http.StatusTooManyRequests, Response{Code: 30003, Message: "too many requests", Data: nil})
	}
}

// redirect answers every lookup miss with the same 404 so the route reveals
// nothing about which URL segment was wrong.
func (h *randomHandler) redirect(c *gin.Context) {
	// Each response is a fresh draw; no cache may pin one image to the link.
	c.Header("Cache-Control", "no-store")
	uid, token := c.Param("uid"), c.Param("token")
	if !service.ValidRandomSegment(uid, randomPublicIDLength) || !service.ValidRandomSegment(token, randomTokenLength) {
		fail(c, service.ErrNotFound)
		return
	}
	original := false
	switch c.Query("format") {
	case "":
	case "original":
		original = true
	default:
		fail(c, service.ErrInvalidInput)
		return
	}
	target, err := h.links.Pick(c.Request.Context(), uid, token, original)
	if err != nil {
		fail(c, err)
		return
	}
	c.Header("Location", target)
	c.Status(http.StatusTemporaryRedirect)
}

func (h *randomHandler) get(c *gin.Context) {
	id, ok := albumID(c)
	if !ok {
		return
	}
	view, err := h.links.Get(c.Request.Context(), identity(c).User.ID, id)
	if err != nil {
		fail(c, err)
		return
	}
	if view == nil {
		respond(c, 200, nil)
		return
	}
	respond(c, 200, view)
}

func (h *randomHandler) put(c *gin.Context) {
	id, ok := albumID(c)
	if !ok {
		return
	}
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if !decode(c, &input) {
		return
	}
	if input.Enabled == nil {
		fail(c, service.ErrInvalidInput)
		return
	}
	view, err := h.links.Put(c.Request.Context(), identity(c).User.ID, id, *input.Enabled)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, view)
}

func (h *randomHandler) reset(c *gin.Context) {
	id, ok := albumID(c)
	if !ok {
		return
	}
	view, err := h.links.Reset(c.Request.Context(), identity(c).User.ID, id)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, view)
}

func (h *randomHandler) remove(c *gin.Context) {
	id, ok := albumID(c)
	if !ok {
		return
	}
	if err := h.links.Delete(c.Request.Context(), identity(c).User.ID, id); err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, nil)
}
