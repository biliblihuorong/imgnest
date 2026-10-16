package native

import (
	"context"
	"fmt"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// EmailAuth sends registration and password-reset codes.
type EmailAuth interface {
	EmailCodeRequired(context.Context) (bool, error)
	SendEmailCode(ctx context.Context, purpose, email string) error
	ResetPasswordWithCode(ctx context.Context, email, code, next string) error
}

// MailSettings is the administrator SMTP configuration API.
type MailSettings interface {
	Config(context.Context, service.UserView) (service.MailConfigView, error)
	SaveConfig(context.Context, service.UserView, service.MailConfigInput) (service.MailConfigView, error)
	Test(ctx context.Context, actor service.UserView, to string) error
}

// RegisterEmailRoutes installs email codes, password reset and the SMTP
// settings. Call during router construction, before serving any requests.
func (h *Handler) RegisterEmailRoutes(ctx context.Context, router gin.IRouter, email EmailAuth, mail MailSettings) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("register email routes: %w", err)
	}
	if email == nil || mail == nil {
		return fmt.Errorf("register email routes: missing dependency")
	}
	h.email = email
	h.mail = mail
	router.POST("/api/auth/email-code", h.rateLimit, h.sendEmailCode)
	router.POST("/api/auth/password-reset", h.rateLimit, h.resetPassword)
	admin := router.Group("/api/admin/mail", h.authenticate, h.requireAdmin)
	admin.GET("", h.getMail)
	admin.PUT("", h.saveMail)
	admin.POST("/test", h.rateLimit, h.testMail)
	return nil
}

func (h *Handler) requireAdmin(c *gin.Context) {
	user := identity(c).User
	if user.Role != model.UserRoleAdmin || user.Status != model.UserStatusEnabled {
		fail(c, service.ErrForbidden)
		c.Abort()
	}
}

// sendEmailCode mails a code once the visitor passed the CAPTCHA for the
// form the code belongs to.
func (h *Handler) sendEmailCode(c *gin.Context) {
	var in struct {
		Email        string `json:"email"`
		Purpose      string `json:"purpose"`
		CaptchaToken string `json:"captcha_token,omitempty"`
	}
	if !decode(c, &in) {
		return
	}
	action := map[string]string{model.EmailCodeRegister: "register", model.EmailCodeResetPassword: "reset"}[in.Purpose]
	if action == "" {
		fail(c, service.ErrInvalidInput)
		return
	}
	if !h.verifyCaptcha(c, in.CaptchaToken, action) {
		return
	}
	if err := h.email.SendEmailCode(c.Request.Context(), in.Purpose, in.Email); err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, nil)
}

func (h *Handler) resetPassword(c *gin.Context) {
	var in struct {
		Email       string `json:"email"`
		Code        string `json:"code"`
		NewPassword string `json:"new_password"`
	}
	if !decode(c, &in) {
		return
	}
	if err := h.email.ResetPasswordWithCode(c.Request.Context(), in.Email, in.Code, in.NewPassword); err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, nil)
}

func (h *Handler) getMail(c *gin.Context) {
	view, err := h.mail.Config(c.Request.Context(), identity(c).User)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, view)
}

func (h *Handler) saveMail(c *gin.Context) {
	var in service.MailConfigInput
	if !decode(c, &in) {
		return
	}
	view, err := h.mail.SaveConfig(c.Request.Context(), identity(c).User, in)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, view)
}

func (h *Handler) testMail(c *gin.Context) {
	var in struct {
		To string `json:"to"`
	}
	if !decode(c, &in) {
		return
	}
	if err := h.mail.Test(c.Request.Context(), identity(c).User, in.To); err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, nil)
}
