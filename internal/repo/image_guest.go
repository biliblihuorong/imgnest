package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// guestLockKey serializes guest quota transitions on PostgreSQL the same way
// the per-user row lock serializes account uploads.
const guestLockKey = 1229801283

// GuestUploadGroup resolves the guest group without requiring any of its
// rules to be usable, for the pre-read of a multipart body whose selected
// rule is not known yet.
func (r *PolicyRepository) GuestUploadGroup(ctx context.Context) (model.Group, error) {
	var group model.Group
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		resolved, ok, err := resolveGuestGroup(ctx, tx)
		if err != nil {
			return err
		}
		if !ok {
			return model.ErrForbidden
		}
		group = resolved
		return nil
	})
	if err != nil {
		return model.Group{}, imageError("resolve guest upload group", err)
	}
	return group, nil
}

// GuestUploadPolicy resolves an enabled rule for the guest group without
// reading or locking any user row. policyID zero selects the group default.
func (r *PolicyRepository) GuestUploadPolicy(ctx context.Context, policyID uint64) (model.Policy, model.Storage, model.Group, error) {
	var policy model.Policy
	var backend model.Storage
	var group model.Group
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		resolved, ok, err := resolveGuestGroup(ctx, tx)
		if err != nil {
			return err
		}
		if !ok {
			return model.ErrForbidden
		}
		policy, backend, group, err = permittedPolicy(tx, model.User{GroupID: resolved.ID}, policyID)
		return err
	})
	if err != nil {
		return model.Policy{}, model.Storage{}, model.Group{}, imageError("resolve guest upload policy", err)
	}
	return policy, backend, group, nil
}

// GuestUsedBytes sums occupied and reserved guest bytes for preflight checks.
func (r *ImageRepository) GuestUsedBytes(ctx context.Context) (int64, error) {
	var occupied, reserved int64
	if err := r.db.WithContext(ctx).Model(&model.Image{}).
		Where("user_id = ? AND state = ?", 0, model.ImageStateActive).
		Select("COALESCE(SUM(charged_bytes),0)").Scan(&occupied).Error; err != nil {
		return 0, databaseError("sum guest bytes", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.Image{}).
		Where("user_id = ? AND (state = ? OR (state = ? AND operation = ?))", 0, model.ImageStatePending, model.ImageStateTrash, model.ImageOperationRestore).
		Select("COALESCE(SUM(charged_bytes),0)").Scan(&reserved).Error; err != nil {
		return 0, databaseError("sum guest bytes", err)
	}
	if occupied < 0 || reserved < 0 || occupied > math.MaxInt64-reserved {
		return 0, model.ErrQuotaExceeded
	}
	return occupied + reserved, nil
}

// ReserveGuestUpload claims a path and quota for a guest's prepared objects.
// It mirrors ReserveUpload: the same path-uniqueness, resumability, group and
// format checks apply, but no user row is read or locked and the quota is
// measured against the guest group with user_id = 0.
func (r *ImageRepository) ReserveGuestUpload(ctx context.Context, req model.UploadReservation) (model.Image, error) {
	if err := ctx.Err(); err != nil {
		return model.Image{}, fmt.Errorf("reserve guest upload: %w", err)
	}
	if req.Grant.UserID != 0 || req.Image.UserID != 0 {
		return model.Image{}, fmt.Errorf("reserve guest upload: %w", model.ErrForbidden)
	}
	image, err := preparedImage(req)
	if err != nil {
		return model.Image{}, imageError("prepare guest reservation", err)
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockGuestQuota(ctx, tx); err != nil {
			return err
		}
		resolved, ok, err := resolveGuestGroup(ctx, tx)
		if err != nil {
			return err
		}
		if !ok {
			return model.ErrForbidden
		}
		policy, backend, group, err := permittedPolicy(tx, model.User{GroupID: resolved.ID}, image.PolicyID)
		if err != nil {
			return err
		}
		if backend.ID != image.StorageID {
			return model.ErrForbidden
		}
		image.PolicyID = policy.ID
		if err := checkUploadGroup(image, req.SourceExt, group); err != nil {
			return err
		}
		if image.AlbumID != 0 {
			return model.ErrForbidden
		}
		var existing model.Image
		lookup := tx.First(&existing, "key = ?", image.Key).Error
		if lookup == nil {
			same := existing.UserID == image.UserID && existing.OperationID == image.OperationID &&
				existing.StorageID == image.StorageID && existing.PolicyID == image.PolicyID && existing.Path == image.Path &&
				existing.SrcMD5 == image.SrcMD5 && existing.ChargedBytes == image.ChargedBytes
			resumable := existing.State == model.ImageStateActive ||
				(existing.State == model.ImageStatePending && existing.Operation == model.ImageOperationUpload)
			if !same || !resumable {
				return model.ErrImageBusy
			}
			image = existing
			return nil
		}
		if !errors.Is(lookup, gorm.ErrRecordNotFound) {
			return lookup
		}
		if err := checkGuestQuota(tx, group, image.ChargedBytes); err != nil {
			return err
		}
		if err := tx.Create(&image).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return model.ErrPathConflict
			}
			return err
		}
		return nil
	})
	if err != nil {
		return model.Image{}, imageError("reserve guest upload", err)
	}
	return image, nil
}

// CommitGuestUpload activates a reserved guest image and commits its metadata.
// Guest usage is derived from images.user_id = 0, so no user row is updated.
func (r *ImageRepository) CommitGuestUpload(ctx context.Context, key, op string, exif model.ImageExif) (model.Image, error) {
	var result model.Image
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockGuestQuota(ctx, tx); err != nil {
			return err
		}
		var image model.Image
		if err := tx.WithContext(ctx).First(&image, "key = ?", key).Error; err != nil {
			return err
		}
		if image.UserID != 0 {
			return model.ErrForbidden
		}
		if op == "" || image.OperationID != op {
			return model.ErrImageBusy
		}
		if image.State == model.ImageStateActive && image.Operation == "" {
			result = image
			return nil
		}
		if image.State != model.ImageStatePending || image.Operation != model.ImageOperationUpload {
			return model.ErrImageBusy
		}
		resolved, ok, err := resolveGuestGroup(ctx, tx)
		if err != nil {
			return err
		}
		if !ok {
			return model.ErrForbidden
		}
		_, backend, group, err := permittedPolicy(tx, model.User{GroupID: resolved.ID}, image.PolicyID)
		if err != nil {
			return err
		}
		if backend.ID != image.StorageID {
			return model.ErrForbidden
		}
		if err := checkUploadGroup(image, "", group); err != nil {
			return err
		}
		if err := checkGuestQuota(tx, group, 0); err != nil {
			return err
		}
		if len(exif.Raw) == 0 {
			exif.Raw = json.RawMessage("{}")
		}
		if !json.Valid(exif.Raw) {
			return model.ErrInvalidInput
		}
		exif.ImageID = image.ID
		if err := tx.Create(&exif).Error; err != nil {
			return err
		}
		if err := adjustAlbum(tx, image, 1); err != nil {
			return err
		}
		if err := updateImage(tx, image.ID, map[string]any{"state": model.ImageStateActive, "operation": ""}); err != nil {
			return err
		}
		image.State = model.ImageStateActive
		image.Operation = ""
		result = image
		return nil
	})
	if err != nil {
		return model.Image{}, imageError("commit guest upload", err)
	}
	return result, nil
}

// lockGuestQuota serializes concurrent guest reservations on PostgreSQL;
// SQLite writes are already serialized by the immediate-transaction DSN.
func lockGuestQuota(ctx context.Context, tx *gorm.DB) error {
	if tx.Name() != "postgres" {
		return nil
	}
	return tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtext(current_schema()), ?)", guestLockKey).Error
}

// checkGuestQuota mirrors checkQuota for the shared guest account: occupied
// active bytes plus reserved pending bytes must stay within the group.
func checkGuestQuota(tx *gorm.DB, group model.Group, additional int64) error {
	var occupied, reserved int64
	if err := tx.Model(&model.Image{}).
		Where("user_id = ? AND state = ?", 0, model.ImageStateActive).
		Select("COALESCE(SUM(charged_bytes),0)").Scan(&occupied).Error; err != nil {
		return err
	}
	if err := tx.Model(&model.Image{}).
		Where("user_id = ? AND (state = ? OR (state = ? AND operation = ?))", 0, model.ImageStatePending, model.ImageStateTrash, model.ImageOperationRestore).
		Select("COALESCE(SUM(charged_bytes),0)").Scan(&reserved).Error; err != nil {
		return err
	}
	if additional < 0 || occupied < 0 || reserved < 0 {
		return model.ErrInvalidInput
	}
	if occupied > math.MaxInt64-reserved {
		return model.ErrQuotaExceeded
	}
	total := occupied + reserved
	if additional > math.MaxInt64-total {
		return model.ErrQuotaExceeded
	}
	if group.CapacityBytes > 0 && (total > group.CapacityBytes || additional > group.CapacityBytes-total) {
		return model.ErrQuotaExceeded
	}
	return nil
}
