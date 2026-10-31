package native

import (
	"context"
	"fmt"
	"strconv"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// AuditLogs reads the operation audit trail.
type AuditLogs interface {
	AuditLogs(context.Context, model.AuditQuery) (service.AuditPage, error)
}

// RegisterAuditRoutes binds the console's operation log.
func (h *Handler) RegisterAuditRoutes(ctx context.Context, router gin.IRouter, logs AuditLogs) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("register audit routes: %w", err)
	}
	if logs == nil {
		return fmt.Errorf("register audit routes: missing dependencies")
	}
	pages := &adminHandler{}
	router.GET("/api/admin/audit-logs", h.authenticate, adminOnly, func(c *gin.Context) {
		page, size, ok := pages.adminPage(c, 50)
		if !ok {
			return
		}
		query := model.AuditQuery{Page: page, Size: size, FailedOnly: c.Query("failed") == "true"}
		if raw, exists := c.GetQuery("user_id"); exists {
			id, err := strconv.ParseUint(raw, 10, 64)
			if err != nil || id == 0 {
				fail(c, service.ErrInvalidInput)
				return
			}
			query.UserID = id
		}
		result, err := logs.AuditLogs(c.Request.Context(), query)
		if err != nil {
			fail(c, err)
			return
		}
		respond(c, 200, result)
	})
	return nil
}
