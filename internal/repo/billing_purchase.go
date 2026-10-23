package repo

import (
	"context"
	"errors"
	"github.com/biliblihuorong/imgnest/extension"
	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

// Purchase commits the immutable order, debit, lease and group transition together.
func (r *BillingRepository) Purchase(ctx context.Context, a extension.BillingActor, in extension.BillingPurchase) (out extension.BillingOrder, err error) {
	if in.PlanID == 0 || in.PlanVersion == 0 || !billingRequestID(in.RequestID) {
		return out, model.ErrInvalidInput
	}
	// Expiry must commit even if a subsequent purchase fails or needs a new login.
	if err = r.Expire(ctx, a.UserID); err != nil {
		return out, err
	}
	err = r.transact(ctx, func(tx *gorm.DB) error {
		user, _, e := r.actor(tx, a, false, 0)
		if e != nil {
			return e
		}
		if user.Role != model.UserRoleUser {
			return model.ErrForbidden
		}
		found, e := billingReplay(tx, a.UserID, "purchase", in.RequestID, in, &out)
		if e != nil || found {
			return e
		}
		var plan billingPlan
		if e = billingLock(tx).First(&plan, "id = ?", in.PlanID).Error; e != nil {
			return e
		}
		if plan.Archived || !plan.Published || plan.Version != in.PlanVersion {
			return extension.ErrBillingConflict
		}
		group, e := lockGroup(ctx, tx, plan.GroupID)
		if e != nil {
			return e
		}
		if group.IsGuest {
			return model.ErrInvalidInput
		}
		now := r.now().UTC()
		var lease billingMembership
		e = tx.First(&lease, "user_id = ?", user.ID).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		current := lease.Status == "active" && user.GroupID == lease.GroupID && now.Before(lease.ExpiresAt)
		start := now
		base := user.GroupID
		var previous *time.Time
		if current {
			if lease.PlanID != plan.ID {
				return extension.ErrBillingConflict
			}
			start = lease.ExpiresAt
			base = lease.BaseGroupID
			old := lease.ExpiresAt
			previous = &old
		}
		expiry := start.Add(time.Duration(plan.DurationDays) * 24 * time.Hour)
		if expiry.Year() > 9999 {
			return model.ErrInvalidInput
		}
		wallet, e := r.wallet(tx, user)
		if e != nil {
			return e
		}
		if wallet.BalanceCents < plan.PriceCents {
			return extension.ErrBillingInsufficient
		}
		order := billingOrder{UserID: user.ID, Username: user.Username, PlanID: plan.ID, PlanName: plan.Name, PlanVersion: plan.Version, PriceCents: plan.PriceCents, DurationDays: plan.DurationDays, GroupID: plan.GroupID, PreviousExpiresAt: previous, ExpiresAt: expiry, RequiresLogin: user.GroupID != plan.GroupID, RequestID: in.RequestID, CreatedAt: now}
		if e = tx.Create(&order).Error; e != nil {
			return e
		}
		if _, e = r.appendEntry(tx, user, user, wallet, -plan.PriceCents, "purchase", plan.Name, &order.ID, nil); e != nil {
			return e
		}
		next := billingMembership{UserID: user.ID, PlanID: plan.ID, GroupID: plan.GroupID, BaseGroupID: base, StartsAt: now, ExpiresAt: expiry, Status: "active", UpdatedAt: now}
		if current {
			next.StartsAt = lease.StartsAt
		}
		if e = tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}}, UpdateAll: true}).Create(&next).Error; e != nil {
			return e
		}
		if e = billingChangeGroup(tx, user, plan.GroupID); e != nil {
			return e
		}
		out = extension.BillingOrder(order)
		return r.recordRequest(tx, user.ID, "purchase", in.RequestID, in, out)
	})
	return out, err
}
