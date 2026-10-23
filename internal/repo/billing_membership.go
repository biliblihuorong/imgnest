package repo

import (
	"context"
	"errors"
	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
	"math"
	"time"
)

func billingChangeGroup(tx *gorm.DB, user model.User, group uint64) error {
	if user.GroupID == group {
		return nil
	}
	if user.AuthVersion >= math.MaxInt64 {
		return model.ErrInvalidInput
	}
	if e := tx.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]any{"group_id": group, "auth_version": gorm.Expr("auth_version + 1")}).Error; e != nil {
		return e
	}
	return tx.Where("user_id = ?", user.ID).Delete(&model.Token{}).Error
}

// Expire settles one user or one bounded batch without requiring a paid plugin.
func (r *BillingRepository) Expire(ctx context.Context, userID uint64) error {
	if userID != 0 {
		return settleBillingUser(ctx, r.db, userID, r.now)
	}
	var users []uint64
	if err := r.db.WithContext(ctx).Model(&billingMembership{}).Where("status = ? AND expires_at <= ?", "active", r.now().UTC()).Order("expires_at ASC, user_id ASC").Limit(200).Pluck("user_id", &users).Error; err != nil {
		return repositoryError("list expired memberships", err)
	}
	var failures []error
	for _, id := range users {
		if err := settleBillingUser(ctx, r.db, id, r.now); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
func settleBillingUser(ctx context.Context, db *gorm.DB, id uint64, now func() time.Time) error {
	if id == 0 {
		return nil
	}
	var due int64
	if err := db.WithContext(ctx).Model(&billingMembership{}).Where("user_id = ? AND status = ? AND expires_at <= ?", id, "active", now().UTC()).Count(&due).Error; err != nil {
		return repositoryError("check membership expiry", err)
	}
	if due == 0 {
		return nil
	}
	r := &BillingRepository{db: db, now: now}
	return r.transact(ctx, func(tx *gorm.DB) error {
		user, err := lockUser(ctx, tx, id)
		if err != nil {
			return err
		}
		var lease billingMembership
		err = tx.First(&lease, "user_id = ?", id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if lease.Status != "active" || now().UTC().Before(lease.ExpiresAt) {
			return nil
		}
		state := "expired"
		if user.GroupID != lease.GroupID || user.Role != model.UserRoleUser {
			state = "overridden"
		} else {
			group, e := lockGroup(ctx, tx, lease.BaseGroupID)
			if e != nil {
				return e
			}
			if group.IsGuest {
				return model.ErrForbidden
			}
			if e = billingChangeGroup(tx, user, lease.BaseGroupID); e != nil {
				return e
			}
		}
		return tx.Model(&billingMembership{}).Where("user_id = ? AND status = ?", id, "active").Updates(map[string]any{"status": state, "updated_at": now().UTC()}).Error
	})
}
func billingGrantCurrent(tx *gorm.DB, id uint64, at time.Time) error {
	var due int64
	if err := tx.Model(&billingMembership{}).Where("user_id = ? AND status = ? AND expires_at <= ?", id, "active", at).Count(&due).Error; err != nil {
		return err
	}
	if due != 0 {
		return model.ErrUnauthenticated
	}
	return nil
}
