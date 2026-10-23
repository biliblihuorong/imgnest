package repo

import (
	"context"
	"github.com/biliblihuorong/imgnest/extension"
	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
	"math"
)

// SavePlan updates an optimistic version; sold group and duration are immutable.
func (r *BillingRepository) SavePlan(ctx context.Context, a extension.BillingActor, in extension.BillingPlanInput) (out extension.BillingPlan, err error) {
	if !billingText(in.Name, 1, 100) || !billingText(in.Description, 0, 2000) || in.PriceCents < 1 || in.PriceCents > extension.MaxBillingCents || in.DurationDays < 1 || in.DurationDays > 3650 || in.GroupID == 0 || in.Published && in.Archived {
		return out, model.ErrInvalidInput
	}
	err = r.transact(ctx, func(tx *gorm.DB) error {
		if _, _, e := r.actor(tx, a, true, 0); e != nil {
			return e
		}
		var old billingPlan
		if in.ID != 0 {
			if e := billingLock(tx).First(&old, "id = ?", in.ID).Error; e != nil {
				return e
			}
			if old.Version != in.Version || old.Version >= math.MaxInt64 || old.Archived && !in.Archived {
				return extension.ErrBillingConflict
			}
			if old.GroupID != in.GroupID || old.DurationDays != in.DurationDays {
				var sold int64
				if e := tx.Model(&billingOrder{}).Where("plan_id = ?", old.ID).Count(&sold).Error; e != nil {
					return e
				}
				if sold != 0 {
					return extension.ErrBillingConflict
				}
			}
		}
		group, e := lockGroup(ctx, tx, in.GroupID)
		if e != nil {
			return e
		}
		if group.IsGuest {
			return model.ErrInvalidInput
		}
		now := r.now().UTC()
		p := billingPlan{ID: in.ID, Name: in.Name, Description: in.Description, PriceCents: in.PriceCents, DurationDays: in.DurationDays, GroupID: in.GroupID, Published: in.Published, Archived: in.Archived, Version: old.Version + 1, CreatedAt: old.CreatedAt, UpdatedAt: now}
		if in.ID == 0 {
			p.CreatedAt = now
			e = tx.Create(&p).Error
		} else {
			e = tx.Model(&billingPlan{}).Where("id = ?", p.ID).Select("name", "description", "price_cents", "duration_days", "group_id", "published", "archived", "version", "updated_at").Updates(&p).Error
		}
		if e != nil {
			return e
		}
		out = extension.BillingPlan(p)
		return nil
	})
	return out, err
}

// Plans returns current public offers or the complete administrator catalog.
func (r *BillingRepository) Plans(ctx context.Context, a extension.BillingActor, admin bool) (out []extension.BillingPlan, err error) {
	out = []extension.BillingPlan{}
	err = r.transact(ctx, func(tx *gorm.DB) error {
		if _, _, e := r.actor(tx, a, admin, 0); e != nil {
			return e
		}
		query := tx.Model(&billingPlan{})
		if !admin {
			query = query.Where("published = ? AND archived = ?", true, false)
		}
		if e := query.Order("id ASC").Find(&out).Error; e != nil {
			return e
		}
		for i := range out {
			var group model.Group
			if e := tx.First(&group, "id = ?", out[i].GroupID).Error; e != nil {
				return e
			}
			out[i].Group = &extension.BillingGroup{Name: group.Name, CapacityBytes: group.CapacityBytes, MaxFileBytes: group.MaxFileBytes, UploadPerMin: group.UploadPerMin}
		}
		return nil
	})
	return out, err
}
