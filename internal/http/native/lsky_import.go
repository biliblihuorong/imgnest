package native

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// LskyImport is the console's Lsky Pro migration: upload an export, then
// import it into a rule in the background.
type LskyImport interface {
	LoadDump(context.Context, io.Reader, string) (service.LskyImportStatus, error)
	Discard(context.Context) (service.LskyImportStatus, error)
	Status(context.Context) (service.LskyImportStatus, error)
	Start(context.Context, service.LskyImportOptions) (service.LskyImportStatus, error)
	Cancel(context.Context) (service.LskyImportStatus, error)
}

// RegisterLskyImportRoutes binds /api/admin/migrations/lsky for administrators.
func (h *Handler) RegisterLskyImportRoutes(ctx context.Context, router gin.IRouter, importer LskyImport) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("register lsky import routes: %w", err)
	}
	if importer == nil {
		return service.ErrInvalidInput
	}
	handler := &lskyImportHandler{importer: importer}
	group := router.Group("/api/admin/migrations/lsky", h.authenticate, requireAdminRole)
	group.GET("", handler.status)
	group.DELETE("", handler.discard)
	group.POST("/dump", handler.upload)
	group.POST("/start", handler.start)
	group.POST("/cancel", handler.cancel)
	return nil
}

func requireAdminRole(c *gin.Context) {
	if identity(c).User.Role != model.UserRoleAdmin {
		fail(c, service.ErrForbidden)
		c.Abort()
	}
}

type lskyImportHandler struct{ importer LskyImport }

func (h *lskyImportHandler) reply(c *gin.Context, status service.LskyImportStatus, err error) {
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, status)
}

func (h *lskyImportHandler) status(c *gin.Context) {
	status, err := h.importer.Status(c.Request.Context())
	h.reply(c, status, err)
}

func (h *lskyImportHandler) discard(c *gin.Context) {
	status, err := h.importer.Discard(c.Request.Context())
	h.reply(c, status, err)
}

func (h *lskyImportHandler) cancel(c *gin.Context) {
	status, err := h.importer.Cancel(c.Request.Context())
	h.reply(c, status, err)
}

func (h *lskyImportHandler) start(c *gin.Context) {
	var input service.LskyImportOptions
	if !decode(c, &input) {
		return
	}
	status, err := h.importer.Start(c.Request.Context(), input)
	h.reply(c, status, err)
}

// upload streams the "file" part of a multipart body straight into the
// parser, so the export (with its password hashes) never lands in a
// temporary file.
func (h *lskyImportHandler) upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.LskyMaxDumpBytes+1<<20)
	reader, err := c.Request.MultipartReader()
	if err != nil {
		fail(c, service.ErrInvalidInput)
		return
	}
	zone := c.DefaultQuery("timezone", "Asia/Shanghai")
	for {
		part, err := reader.NextPart()
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				fail(c, errUploadTooLarge)
				return
			}
			fail(c, service.ErrInvalidInput)
			return
		}
		if part.FormName() != "file" {
			_ = part.Close()
			continue
		}
		status, err := h.importer.LoadDump(c.Request.Context(), part, zone)
		_ = part.Close()
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			err = errUploadTooLarge
		}
		h.reply(c, status, err)
		return
	}
}
