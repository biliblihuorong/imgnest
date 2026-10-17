package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// LoginHistoryLimit is how many of the newest sign-ins are kept per account.
const LoginHistoryLimit = 20

// recordLogin stores one sign-in, refreshes the account's last-seen time and
// prunes sign-ins beyond the kept history, inside the caller's transaction.
func recordLogin(tx *gorm.DB, userID uint64, event model.LoginEvent) error {
	at := event.CreatedAt.UTC()
	row := model.LoginEvent{UserID: userID, IP: event.IP, UserAgent: event.UserAgent, CreatedAt: at}
	if err := tx.Create(&row).Error; err != nil {
		return err
	}
	if err := tx.Model(&model.User{}).Where("id = ?", userID).UpdateColumn("last_seen_at", at).Error; err != nil {
		return err
	}
	return tx.Exec(
		"DELETE FROM login_events WHERE user_id = ? AND id NOT IN "+
			"(SELECT id FROM login_events WHERE user_id = ? ORDER BY id DESC LIMIT ?)",
		userID, userID, LoginHistoryLimit,
	).Error
}

// ListLoginEvents returns an account's newest sign-ins first.
func (r *TokenRepository) ListLoginEvents(ctx context.Context, userID uint64, limit int) ([]model.LoginEvent, error) {
	if err := checkRecordID(ctx, userID); err != nil {
		return nil, fmt.Errorf("list login events: %w", err)
	}
	events := []model.LoginEvent{}
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("id DESC").Limit(limit).Find(&events).Error; err != nil {
		return nil, repositoryError("list login events", err)
	}
	for i := range events {
		events[i].CreatedAt = events[i].CreatedAt.UTC()
	}
	return events, nil
}

// ListAPIUsage returns an account's daily API-token request totals from the
// given UTC day onward, oldest first.
func (r *TokenRepository) ListAPIUsage(ctx context.Context, userID uint64, since time.Time) ([]model.APIUsageDay, error) {
	if err := checkRecordID(ctx, userID); err != nil {
		return nil, fmt.Errorf("list api usage: %w", err)
	}
	days := []model.APIUsageDay{}
	err := r.db.WithContext(ctx).Table("api_usage_daily").
		Where("user_id = ? AND day >= ?", userID, since.UTC().Format(time.DateOnly)).
		Order("day ASC").Find(&days).Error
	if err != nil {
		return nil, repositoryError("list api usage", err)
	}
	return days, nil
}
