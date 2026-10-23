package repo

import (
	"context"
	"errors"
	"github.com/biliblihuorong/imgnest/extension"
	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math"
)

func (r *BillingRepository) wallet(tx *gorm.DB, u model.User) (billingWallet, error) {
	now := r.now().UTC()
	w := billingWallet{UserID: u.ID, Username: u.Username, UpdatedAt: now}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&w).Error; err != nil {
		return w, err
	}
	err := billingLock(tx).First(&w, "user_id = ?", u.ID).Error
	return w, err
}
func (r *BillingRepository) appendEntry(tx *gorm.DB, actor, owner model.User, w billingWallet, amount int64, kind, reason string, order, reverse *uint64) (extension.BillingEntry, error) {
	if amount == 0 || amount > extension.MaxBillingCents || amount < -extension.MaxBillingCents || w.Version >= math.MaxInt64 {
		return extension.BillingEntry{}, model.ErrInvalidInput
	}
	next := w.BalanceCents + amount
	if next < 0 {
		return extension.BillingEntry{}, extension.ErrBillingInsufficient
	}
	if next > extension.MaxBillingCents {
		return extension.BillingEntry{}, model.ErrInvalidInput
	}
	now := r.now().UTC()
	e := billingEntry{UserID: owner.ID, Username: owner.Username, Sequence: w.Version + 1, AmountCents: amount, BeforeCents: w.BalanceCents, AfterCents: next, Kind: kind, Reason: reason, ActorID: actor.ID, ActorName: actor.Username, OrderID: order, ReversesID: reverse, CreatedAt: now}
	if err := tx.Create(&e).Error; err != nil {
		return extension.BillingEntry{}, err
	}
	update := tx.Model(&billingWallet{}).Where("user_id = ? AND version = ?", owner.ID, w.Version).Updates(map[string]any{"balance_cents": next, "version": w.Version + 1, "username": owner.Username, "updated_at": now})
	if update.Error != nil {
		return extension.BillingEntry{}, update.Error
	}
	if update.RowsAffected != 1 {
		return extension.BillingEntry{}, extension.ErrBillingConflict
	}
	return extension.BillingEntry(e), nil
}

// Overview returns the caller's private balance and current or last membership.
func (r *BillingRepository) Overview(ctx context.Context, a extension.BillingActor) (out extension.BillingOverview, err error) {
	out.Currency = "CNY"
	err = r.transact(ctx, func(tx *gorm.DB) error {
		u, _, e := r.actor(tx, a, false, 0)
		if e != nil {
			return e
		}
		var w billingWallet
		e = tx.First(&w, "user_id = ?", u.ID).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			w = billingWallet{UserID: u.ID, Username: u.Username}
		} else if e != nil {
			return e
		}
		out.Wallet = extension.BillingWallet(w)
		out.Membership = nil
		var m billingMembership
		e = tx.First(&m, "user_id = ?", u.ID).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return nil
		}
		if e != nil {
			return e
		}
		v := extension.BillingMembership(m)
		out.Membership = &v
		return nil
	})
	return out, err
}

// Adjust appends a reasoned administrator credit/debit and its replay result.
func (r *BillingRepository) Adjust(ctx context.Context, a extension.BillingActor, in extension.BillingAdjustment) (out extension.BillingEntry, err error) {
	if in.UserID == 0 || in.AmountCents == 0 || in.AmountCents > extension.MaxBillingCents || in.AmountCents < -extension.MaxBillingCents || !billingText(in.Reason, 1, 500) || !billingRequestID(in.RequestID) {
		return out, model.ErrInvalidInput
	}
	err = r.transact(ctx, func(tx *gorm.DB) error {
		actor, owner, e := r.actor(tx, a, true, in.UserID)
		if e != nil {
			return e
		}
		found, e := billingReplay(tx, a.UserID, "adjust", in.RequestID, in, &out)
		if e != nil || found {
			return e
		}
		if owner.Status != model.UserStatusEnabled {
			return model.ErrForbidden
		}
		w, e := r.wallet(tx, owner)
		if e != nil {
			return e
		}
		if _, _, e = r.actor(tx, a, true, in.UserID); e != nil {
			return e
		}
		kind := "admin_credit"
		if in.AmountCents < 0 {
			kind = "admin_debit"
		}
		out, e = r.appendEntry(tx, actor, owner, w, in.AmountCents, kind, in.Reason, nil, nil)
		if e != nil {
			return e
		}
		return r.recordRequest(tx, a.UserID, "adjust", in.RequestID, in, out)
	})
	return out, err
}

// Reverse adds one complete compensating entry for an administrator adjustment.
func (r *BillingRepository) Reverse(ctx context.Context, a extension.BillingActor, in extension.BillingReversal) (out extension.BillingEntry, err error) {
	if in.EntryID == 0 || !billingText(in.Reason, 1, 500) || !billingRequestID(in.RequestID) {
		return out, model.ErrInvalidInput
	}
	// Immutable lookup determines the target before acquiring ordered user locks.
	var original billingEntry
	if err = r.db.WithContext(ctx).First(&original, "id = ?", in.EntryID).Error; err != nil {
		return out, repositoryError("find reversal entry", err)
	}
	err = r.transact(ctx, func(tx *gorm.DB) error {
		actor, owner, e := r.actor(tx, a, true, original.UserID)
		if e != nil {
			return e
		}
		found, e := billingReplay(tx, a.UserID, "reverse", in.RequestID, in, &out)
		if e != nil || found {
			return e
		}
		if original.Kind != "admin_credit" && original.Kind != "admin_debit" {
			return extension.ErrBillingConflict
		}
		var count int64
		if e = tx.Model(&billingEntry{}).Where("reverses_id = ?", original.ID).Count(&count).Error; e != nil {
			return e
		}
		if count != 0 {
			return extension.ErrBillingConflict
		}
		w, e := r.wallet(tx, owner)
		if e != nil {
			return e
		}
		if _, _, e = r.actor(tx, a, true, original.UserID); e != nil {
			return e
		}
		out, e = r.appendEntry(tx, actor, owner, w, -original.AmountCents, "reversal", in.Reason, nil, &original.ID)
		if e != nil {
			return e
		}
		return r.recordRequest(tx, a.UserID, "reverse", in.RequestID, in, out)
	})
	return out, err
}

// Wallets lists administrator-visible wallet snapshots.
func (r *BillingRepository) Wallets(ctx context.Context, a extension.BillingActor, f extension.BillingFilter) (out extension.BillingPage[extension.BillingWallet], err error) {
	err = r.transact(ctx, func(tx *gorm.DB) error {
		if _, _, e := r.actor(tx, a, true, 0); e != nil {
			return e
		}
		var e error
		out, e = billingPage[extension.BillingWallet](tx, billingWallet{}, f)
		return e
	})
	return out, err
}

// Ledger pages immutable entries; All is accepted only for an administrator.
func (r *BillingRepository) Ledger(ctx context.Context, a extension.BillingActor, f extension.BillingFilter) (out extension.BillingPage[extension.BillingEntry], err error) {
	err = r.transact(ctx, func(tx *gorm.DB) error {
		if _, _, e := r.actor(tx, a, f.All, 0); e != nil {
			return e
		}
		if !f.All {
			f.UserID = a.UserID
		}
		var e error
		out, e = billingPage[extension.BillingEntry](tx, billingEntry{}, f)
		return e
	})
	return out, err
}

// Orders pages committed purchases with immutable terms.
func (r *BillingRepository) Orders(ctx context.Context, a extension.BillingActor, f extension.BillingFilter) (out extension.BillingPage[extension.BillingOrder], err error) {
	err = r.transact(ctx, func(tx *gorm.DB) error {
		if _, _, e := r.actor(tx, a, f.All, 0); e != nil {
			return e
		}
		if !f.All {
			f.UserID = a.UserID
		}
		var e error
		out, e = billingPage[extension.BillingOrder](tx, billingOrder{}, f)
		return e
	})
	return out, err
}

// Memberships lists current and terminal leases for the administrator.
func (r *BillingRepository) Memberships(ctx context.Context, a extension.BillingActor, f extension.BillingFilter) (out extension.BillingPage[extension.BillingMembership], err error) {
	err = r.transact(ctx, func(tx *gorm.DB) error {
		if _, _, e := r.actor(tx, a, true, 0); e != nil {
			return e
		}
		var e error
		out, e = billingPage[extension.BillingMembership](tx, billingMembership{}, f)
		return e
	})
	return out, err
}
