package native

import (
	"context"
	"fmt"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// CaptchaService is the native-only challenge/configuration capability.
type CaptchaService interface {
	Public(context.Context) (service.CaptchaPublicView, error)
	Verify(context.Context, string, string) error
	Admin(context.Context, service.UserView) (service.CaptchaAdminView, error)
	SaveDraft(context.Context, service.UserView, service.CaptchaDraftInput) (service.CaptchaAdminView, error)
	TestDraft(context.Context, service.UserView, service.CaptchaTestInput) (service.CaptchaAdminView, error)
	Activate(context.Context, service.UserView, service.CaptchaActivationInput) (service.CaptchaAdminView, error)
}

// RegisterCaptchaRoutes installs administrator-only settings and the verifier.
// Call during router construction, before serving any requests.
func (h *Handler) RegisterCaptchaRoutes(ctx context.Context, router gin.IRouter, captcha CaptchaService) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("register captcha routes: %w", err)
	}
	if captcha == nil {
		return fmt.Errorf("register captcha routes: missing dependency")
	}
	h.captcha = captcha
	admin := router.Group("/api/admin/captcha", h.authenticate, func(c *gin.Context) {
		user := identity(c).User
		if user.Role != model.UserRoleAdmin || user.Status != model.UserStatusEnabled {
			fail(c, service.ErrForbidden)
			c.Abort()
		}
	})
	admin.GET("", h.getCaptcha)
	admin.PUT("/draft", h.saveCaptchaDraft)
	admin.POST("/test", h.rateLimit, h.testCaptchaDraft)
	admin.POST("/activation", h.activateCaptcha)
	return nil
}
func (h *Handler) publicCaptcha(c *gin.Context) {
	if h.captcha == nil {
		respond(c, 200, service.CaptchaPublicView{})
		return
	}
	view, err := h.captcha.Public(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, view)
}
func (h *Handler) verifyCaptcha(c *gin.Context, token, action string) bool {
	if len(token) > 2048 {
		fail(c, service.ErrCaptchaFailed)
		return false
	}
	if h.captcha == nil {
		return true
	}
	if err := h.captcha.Verify(c.Request.Context(), token, action); err != nil {
		fail(c, err)
		return false
	}
	return true
}
func (h *Handler) getCaptcha(c *gin.Context) {
	view, err := h.captcha.Admin(c.Request.Context(), identity(c).User)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, view)
}
func (h *Handler) saveCaptchaDraft(c *gin.Context) {
	var in struct {
		service.CaptchaDraftInput
		ExpectedVersion *uint64 `json:"expected_version"`
	}
	if !decode(c, &in) {
		return
	}
	if in.ExpectedVersion == nil {
		fail(c, service.ErrInvalidInput)
		return
	}
	in.CaptchaDraftInput.ExpectedVersion = *in.ExpectedVersion
	view, err := h.captcha.SaveDraft(c.Request.Context(), identity(c).User, in.CaptchaDraftInput)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, view)
}
func (h *Handler) testCaptchaDraft(c *gin.Context) {
	var in struct {
		service.CaptchaTestInput
		ExpectedVersion *uint64 `json:"expected_version"`
	}
	if !decode(c, &in) {
		return
	}
	if in.ExpectedVersion == nil {
		fail(c, service.ErrInvalidInput)
		return
	}
	in.CaptchaTestInput.ExpectedVersion = *in.ExpectedVersion
	view, err := h.captcha.TestDraft(c.Request.Context(), identity(c).User, in.CaptchaTestInput)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, view)
}
func (h *Handler) activateCaptcha(c *gin.Context) {
	var in struct {
		ExpectedVersion                  *uint64 `json:"expected_version"`
		Enabled                          *bool   `json:"enabled"`
		AcknowledgeLegacyIncompatibility *bool   `json:"acknowledge_legacy_incompatibility"`
		AcknowledgeV1Unprotected         *bool   `json:"acknowledge_v1_unprotected"`
	}
	if !decode(c, &in) {
		return
	}
	missingDecision := in.ExpectedVersion == nil || in.Enabled == nil
	missingAcknowledgement := in.AcknowledgeLegacyIncompatibility == nil || in.AcknowledgeV1Unprotected == nil
	if missingDecision || missingAcknowledgement {
		fail(c, service.ErrInvalidInput)
		return
	}
	view, err := h.captcha.Activate(c.Request.Context(), identity(c).User, service.CaptchaActivationInput{
		ExpectedVersion: *in.ExpectedVersion, Enabled: *in.Enabled,
		AcknowledgeLegacyIncompatibility: *in.AcknowledgeLegacyIncompatibility,
		AcknowledgeV1Unprotected:         *in.AcknowledgeV1Unprotected,
	})
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, view)
}
