package service

import (
	"context"
	"github.com/biliblihuorong/imgnest/extension"
)

// BillingService delegates commerce only while its optional provider is enabled.
type BillingService struct{ provider extension.BillingProvider }

// NewBillingService constructs the optional account-commerce service.
func NewBillingService(ctx context.Context, p extension.BillingProvider) (*BillingService, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &BillingService{provider: p}, nil
}

// Enabled reports runtime availability; the provider is consulted on each call.
func (s *BillingService) Enabled(ctx context.Context) bool {
	if s.provider == nil {
		return false
	}
	store, ok := s.provider.BillingService(ctx)
	return ok && store != nil
}

// BillingActorFromIdentity preserves the verified session's authorization epoch.
func BillingActorFromIdentity(id Identity) extension.BillingActor {
	a := extension.BillingActor{UserID: id.User.ID, TokenID: id.TokenID}
	if id.Subject.accountState != nil {
		a.AuthVersion = id.Subject.accountState.AuthVersion
	}
	return a
}
func billingCall[T any](ctx context.Context, p extension.BillingProvider, call func(extension.BillingStore) (T, error)) (T, error) {
	var zero T
	if p == nil {
		return zero, extension.ErrBillingUnavailable
	}
	store, ok := p.BillingService(ctx)
	if !ok || store == nil {
		return zero, extension.ErrBillingUnavailable
	}
	return call(store)
}

// Overview forwards the authenticated operation through the runtime gate.
func (s *BillingService) Overview(ctx context.Context, a extension.BillingActor) (extension.BillingOverview, error) {
	return billingCall(ctx, s.provider, func(store extension.BillingStore) (extension.BillingOverview, error) { return store.Overview(ctx, a) })
}

// Plans forwards the authenticated operation through the runtime gate.
func (s *BillingService) Plans(ctx context.Context, a extension.BillingActor, admin bool) ([]extension.BillingPlan, error) {
	return billingCall(ctx, s.provider, func(store extension.BillingStore) ([]extension.BillingPlan, error) { return store.Plans(ctx, a, admin) })
}

// SavePlan forwards the authenticated operation through the runtime gate.
func (s *BillingService) SavePlan(ctx context.Context, a extension.BillingActor, in extension.BillingPlanInput) (extension.BillingPlan, error) {
	return billingCall(ctx, s.provider, func(store extension.BillingStore) (extension.BillingPlan, error) { return store.SavePlan(ctx, a, in) })
}

// Wallets forwards the authenticated operation through the runtime gate.
func (s *BillingService) Wallets(ctx context.Context, a extension.BillingActor, f extension.BillingFilter) (extension.BillingPage[extension.BillingWallet], error) {
	return billingCall(ctx, s.provider, func(store extension.BillingStore) (extension.BillingPage[extension.BillingWallet], error) {
		return store.Wallets(ctx, a, f)
	})
}

// Ledger forwards the authenticated operation through the runtime gate.
func (s *BillingService) Ledger(ctx context.Context, a extension.BillingActor, f extension.BillingFilter) (extension.BillingPage[extension.BillingEntry], error) {
	return billingCall(ctx, s.provider, func(store extension.BillingStore) (extension.BillingPage[extension.BillingEntry], error) {
		return store.Ledger(ctx, a, f)
	})
}

// Orders forwards the authenticated operation through the runtime gate.
func (s *BillingService) Orders(ctx context.Context, a extension.BillingActor, f extension.BillingFilter) (extension.BillingPage[extension.BillingOrder], error) {
	return billingCall(ctx, s.provider, func(store extension.BillingStore) (extension.BillingPage[extension.BillingOrder], error) {
		return store.Orders(ctx, a, f)
	})
}

// Memberships forwards the authenticated operation through the runtime gate.
func (s *BillingService) Memberships(ctx context.Context, a extension.BillingActor, f extension.BillingFilter) (extension.BillingPage[extension.BillingMembership], error) {
	return billingCall(ctx, s.provider, func(store extension.BillingStore) (extension.BillingPage[extension.BillingMembership], error) {
		return store.Memberships(ctx, a, f)
	})
}

// Adjust forwards the authenticated operation through the runtime gate.
func (s *BillingService) Adjust(ctx context.Context, a extension.BillingActor, in extension.BillingAdjustment) (extension.BillingEntry, error) {
	return billingCall(ctx, s.provider, func(store extension.BillingStore) (extension.BillingEntry, error) { return store.Adjust(ctx, a, in) })
}

// Reverse forwards the authenticated operation through the runtime gate.
func (s *BillingService) Reverse(ctx context.Context, a extension.BillingActor, in extension.BillingReversal) (extension.BillingEntry, error) {
	return billingCall(ctx, s.provider, func(store extension.BillingStore) (extension.BillingEntry, error) { return store.Reverse(ctx, a, in) })
}

// Purchase forwards the authenticated operation through the runtime gate.
func (s *BillingService) Purchase(ctx context.Context, a extension.BillingActor, in extension.BillingPurchase) (extension.BillingOrder, error) {
	return billingCall(ctx, s.provider, func(store extension.BillingStore) (extension.BillingOrder, error) { return store.Purchase(ctx, a, in) })
}
