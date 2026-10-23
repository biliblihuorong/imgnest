package cli

import (
	"context"
	"fmt"
	"github.com/biliblihuorong/imgnest/extension"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/service"
	"gorm.io/gorm"
	"time"
)

func newBillingService(ctx context.Context, db *gorm.DB, plugins []extension.Plugin) (*service.BillingService, error) {
	store, err := repo.NewBillingRepository(ctx, db, time.Now)
	if err != nil {
		return nil, err
	}
	var provider extension.BillingProvider
	for _, p := range plugins {
		if receiver, ok := p.(extension.BillingStoreUser); ok {
			receiver.UseBillingStore(store)
		}
		if candidate, ok := p.(extension.BillingProvider); ok {
			if provider != nil {
				return nil, fmt.Errorf("billing: multiple providers")
			}
			provider = candidate
		}
	}
	return service.NewBillingService(ctx, provider)
}
