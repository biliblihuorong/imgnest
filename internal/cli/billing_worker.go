package cli

import (
	"context"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"gorm.io/gorm"
	"log/slog"
	"time"
)

// runBillingExpiry remains active in every edition for previously bought leases.
func runBillingExpiry(ctx context.Context, db *gorm.DB, logger *slog.Logger) {
	store, err := repo.NewBillingRepository(ctx, db, time.Now)
	if err != nil {
		logger.ErrorContext(ctx, "membership expiry unavailable", "code", 50001)
		return
	}
	run := func() {
		job, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := store.Expire(job, 0); err != nil && ctx.Err() == nil {
			logger.ErrorContext(ctx, "membership expiry pending", "code", 50001)
		}
	}
	run()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
