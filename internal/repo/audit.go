package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// AuditRepository stores the operation audit trail.
type AuditRepository struct{ db *gorm.DB }

// NewAuditRepository constructs the audit adapter.
func NewAuditRepository(ctx context.Context, db *gorm.DB) (*AuditRepository, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("create audit repository: %w", err)
	}
	if db == nil {
		return nil, fmt.Errorf("create audit repository: %w", model.ErrInvalidInput)
	}
	return &AuditRepository{db: db}, nil
}

// InsertAuditLogs writes a batch of entries in one statement.
func (r *AuditRepository) InsertAuditLogs(ctx context.Context, entries []model.AuditLog) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("insert audit logs: %w", err)
	}
	if len(entries) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Create(&entries).Error; err != nil {
		return databaseError("insert audit logs", err)
	}
	return nil
}

// ListAuditLogs returns one page of entries, newest first, and the total
// matching the filter.
func (r *AuditRepository) ListAuditLogs(ctx context.Context, query model.AuditQuery) ([]model.AuditLog, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, fmt.Errorf("list audit logs: %w", err)
	}
	if query.Page < 1 || query.Size < 1 {
		return nil, 0, fmt.Errorf("list audit logs: %w", model.ErrInvalidInput)
	}
	if err := checkRecordID(ctx, query.UserID); err != nil {
		return nil, 0, fmt.Errorf("list audit logs: %w", err)
	}
	filtered := r.db.WithContext(ctx).Model(&model.AuditLog{})
	if query.UserID != 0 {
		filtered = filtered.Where("user_id = ?", query.UserID)
	}
	if query.FailedOnly {
		filtered = filtered.Where("status >= 400")
	}
	var total int64
	if err := filtered.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, databaseError("count audit logs", err)
	}
	entries := []model.AuditLog{}
	err := filtered.Session(&gorm.Session{}).Order("id DESC").
		Offset((query.Page - 1) * query.Size).Limit(query.Size).Find(&entries).Error
	if err != nil {
		return nil, 0, databaseError("list audit logs", err)
	}
	for i := range entries {
		entries[i].CreatedAt = entries[i].CreatedAt.UTC()
	}
	return entries, total, nil
}

// PruneAuditLogs deletes entries recorded before the cutoff.
func (r *AuditRepository) PruneAuditLogs(ctx context.Context, before time.Time) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("prune audit logs: %w", err)
	}
	result := r.db.WithContext(ctx).Where("created_at < ?", before.UTC()).Delete(&model.AuditLog{})
	if result.Error != nil {
		return 0, databaseError("prune audit logs", result.Error)
	}
	return result.RowsAffected, nil
}
