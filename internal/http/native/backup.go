package native

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// Backups is the console's backup page: archives of the database and the
// deployment configuration in the backup directory.
type Backups interface {
	Overview(context.Context) (service.BackupOverview, error)
	Create(context.Context) (model.BackupFile, error)
	Open(context.Context, string) (service.BackupArchive, model.BackupFile, error)
	Delete(context.Context, string) error
	PutSchedule(context.Context, service.BackupSchedule) (service.BackupSchedule, error)
}

// RegisterBackupRoutes binds /api/admin/backups for administrators.
func (h *Handler) RegisterBackupRoutes(ctx context.Context, router gin.IRouter, backups Backups) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("register backup routes: %w", err)
	}
	if backups == nil {
		return service.ErrInvalidInput
	}
	handler := &backupHandler{backups: backups}
	group := router.Group("/api/admin/backups", h.authenticate, requireAdminRole)
	group.GET("", handler.overview)
	group.POST("", handler.create)
	group.PUT("/schedule", handler.schedule)
	group.GET("/:name", handler.download)
	group.DELETE("/:name", handler.remove)
	return nil
}

type backupHandler struct{ backups Backups }

func (h *backupHandler) overview(c *gin.Context) {
	overview, err := h.backups.Overview(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, overview)
}

func (h *backupHandler) create(c *gin.Context) {
	file, err := h.backups.Create(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 201, file)
}

func (h *backupHandler) schedule(c *gin.Context) {
	var input service.BackupSchedule
	if !decode(c, &input) {
		return
	}
	schedule, err := h.backups.PutSchedule(c.Request.Context(), input)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, schedule)
}

func (h *backupHandler) remove(c *gin.Context) {
	if err := h.backups.Delete(c.Request.Context(), c.Param("name")); err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, gin.H{"name": c.Param("name")})
}

// download streams the archive as an attachment. Names are validated by the
// service, so the header never carries a path or a quote.
func (h *backupHandler) download(c *gin.Context) {
	archive, file, err := h.backups.Open(c.Request.Context(), c.Param("name"))
	if err != nil {
		fail(c, err)
		return
	}
	defer func() { _ = archive.Close() }()
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Length", strconv.FormatInt(file.Size, 10))
	c.Header("Content-Disposition", `attachment; filename="`+file.Name+`"`)
	c.Status(200)
	_, _ = io.Copy(c.Writer, archive)
}
