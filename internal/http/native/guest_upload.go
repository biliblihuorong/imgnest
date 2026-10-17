package native

import (
	"context"
	"fmt"
	"net/http"

	"github.com/biliblihuorong/imgnest/internal/http/ratelimit"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// GuestImages is the anonymous half of the image service.
type GuestImages interface {
	GuestPreflight(context.Context, uint64) (service.UploadLimits, error)
	GuestPreflightLimits(context.Context) (service.UploadLimits, error)
	GuestUpload(context.Context, service.UploadInput) (service.ImageView, error)
}

// GuestState reports whether the administrator opened guest uploads and the
// guest group exists.
type GuestState interface {
	GuestUploadState(context.Context) (bool, model.Group, bool, error)
}

// RegisterGuestUploadRoutes binds the public upload page's endpoints. They
// share the account upload's slots and limiter; guests spend a per-address
// allowance, cannot pick albums, and their uploads never enter the gallery.
func (h *Handler) RegisterGuestUploadRoutes(ctx context.Context, router gin.IRouter, guests GuestImages, state GuestState) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("register guest upload routes: %w", err)
	}
	if guests == nil || state == nil || h.v2Images == nil {
		return service.ErrInvalidInput
	}
	handler := &guestUploadHandler{images: h.v2Images, guests: guests, state: state}
	router.POST("/api/guest/upload", handler.upload)
	router.GET("/api/guest/upload/limits", handler.limits)
	return nil
}

type guestUploadHandler struct {
	images *imageHandler
	guests GuestImages
	state  GuestState
}

// open refuses every guest request while guest uploads are off, with the
// same 401 an anonymous account upload gets.
func (h *guestUploadHandler) open(ctx context.Context) error {
	enabled, _, ok, err := h.state.GuestUploadState(ctx)
	if err != nil {
		return err
	}
	if !enabled || !ok {
		return service.ErrUnauthenticated
	}
	return nil
}

func (h *guestUploadHandler) upload(c *gin.Context) {
	if err := h.open(c.Request.Context()); err != nil {
		fail(c, err)
		return
	}
	h.images.receive(c, uploadMode{
		limits:    h.guests.GuestPreflightLimits,
		preflight: h.guests.GuestPreflight,
		store:     h.guests.GuestUpload,
		rateKey:   "upload:guest:" + ratelimit.ClientKey(c.ClientIP()),
		guest:     true,
	})
}

func (h *guestUploadHandler) limits(c *gin.Context) {
	ctx := c.Request.Context()
	if err := h.open(ctx); err != nil {
		fail(c, err)
		return
	}
	limits, err := h.guests.GuestPreflightLimits(ctx)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, http.StatusOK, uploadLimitsView{MaxFileBytes: limits.MaxFileBytes, MaxFiles: maxUploadFiles, UploadPerMin: limits.PerMinute})
}
