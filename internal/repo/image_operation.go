package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// BeginTrash records logical deletion and releases charged quota exactly once.
func (r *ImageRepository) BeginTrash(ctx context.Context, key, op string, grant model.TokenGrant, trashDays int) (model.Image, error) {
	if err := ctx.Err(); err != nil {
		return model.Image{}, fmt.Errorf("begin trash: %w", err)
	}
	if op == "" || trashDays < 0 || trashDays > 36500 {
		return model.Image{}, fmt.Errorf("begin trash: %w", model.ErrInvalidInput)
	}
	var result model.Image
	err := r.withImage(ctx, key, &grant, func(tx *gorm.DB, image *model.Image, owner model.User) error {
		repeated := image.State == model.ImageStateTrash && image.OperationID == op &&
			(image.Operation == model.ImageOperationTrash || image.Operation == "")
		if repeated {
			result = *image
			return nil
		}
		if image.State != model.ImageStateActive || image.Operation != "" || image.OperationID == op {
			return model.ErrImageBusy
		}
		if err := chargeOwner(tx, owner, -image.ChargedBytes); err != nil {
			return err
		}
		if err := adjustAlbum(tx, *image, -1); err != nil {
			return err
		}
		deleted := grant.At.UTC()
		purge := deleted.Add(time.Duration(trashDays) * 24 * time.Hour)
		values := map[string]any{
			"state": model.ImageStateTrash, "operation": model.ImageOperationTrash,
			"operation_id": op, "deleted_at": deleted, "purge_at": purge,
		}
		if err := updateImage(tx, image.ID, values); err != nil {
			return err
		}
		image.State = model.ImageStateTrash
		image.Operation = model.ImageOperationTrash
		image.OperationID = op
		image.DeletedAt = &deleted
		image.PurgeAt = &purge
		result = *image
		return nil
	})
	if err != nil {
		return model.Image{}, imageError("begin trash", err)
	}
	return result, nil
}

// FinishTrash confirms removal of the original keys while retaining the reserved path.
func (r *ImageRepository) FinishTrash(ctx context.Context, key, op string) error {
	err := r.withImage(ctx, key, nil, func(tx *gorm.DB, image *model.Image, _ model.User) error {
		if op == "" || image.OperationID != op || image.State != model.ImageStateTrash {
			return model.ErrImageBusy
		}
		if image.Operation == "" {
			return nil
		}
		if image.Operation != model.ImageOperationTrash {
			return model.ErrImageBusy
		}
		return updateImage(tx, image.ID, map[string]any{"operation": ""})
	})
	return finishImageError("finish trash", err)
}

// BeginRestore reserves quota while the image retains its trash path.
func (r *ImageRepository) BeginRestore(ctx context.Context, key, op string, grant model.TokenGrant) (model.Image, error) {
	if err := ctx.Err(); err != nil {
		return model.Image{}, fmt.Errorf("begin restore: %w", err)
	}
	if op == "" {
		return model.Image{}, fmt.Errorf("begin restore: %w", model.ErrInvalidInput)
	}
	var result model.Image
	err := r.withImage(ctx, key, &grant, func(tx *gorm.DB, image *model.Image, owner model.User) error {
		if image.State == model.ImageStateTrash && image.Operation == model.ImageOperationRestore && image.OperationID == op {
			result = *image
			return nil
		}
		if image.State != model.ImageStateTrash || image.Operation != "" || image.OperationID == op {
			return model.ErrImageBusy
		}
		if err := checkOwnerQuota(ctx, tx, owner, image.ChargedBytes); err != nil {
			return err
		}
		if err := updateImage(tx, image.ID, map[string]any{"operation": model.ImageOperationRestore, "operation_id": op}); err != nil {
			return err
		}
		image.Operation = model.ImageOperationRestore
		image.OperationID = op
		result = *image
		return nil
	})
	if err != nil {
		return model.Image{}, imageError("begin restore", err)
	}
	return result, nil
}

// FinishRestore activates the image once and retains durable work to remove trash copies.
func (r *ImageRepository) FinishRestore(ctx context.Context, key, op string, grant model.TokenGrant) (model.Image, error) {
	var result model.Image
	err := r.withImage(ctx, key, &grant, func(tx *gorm.DB, image *model.Image, owner model.User) error {
		if op == "" || image.OperationID != op {
			return model.ErrImageBusy
		}
		activeRetry := image.State == model.ImageStateActive &&
			(image.Operation == model.ImageOperationRestoreCleanup || image.Operation == "")
		if activeRetry {
			result = *image
			return nil
		}
		if image.State != model.ImageStateTrash || image.Operation != model.ImageOperationRestore {
			return model.ErrImageBusy
		}
		if err := checkOwnerQuota(ctx, tx, owner, 0); err != nil {
			return err
		}
		if err := chargeOwner(tx, owner, image.ChargedBytes); err != nil {
			return err
		}
		if err := adjustAlbum(tx, *image, 1); err != nil {
			return err
		}
		values := map[string]any{
			"state": model.ImageStateActive, "operation": model.ImageOperationRestoreCleanup,
			"deleted_at": nil, "purge_at": nil,
		}
		if err := updateImage(tx, image.ID, values); err != nil {
			return err
		}
		image.State = model.ImageStateActive
		image.Operation = model.ImageOperationRestoreCleanup
		image.DeletedAt = nil
		image.PurgeAt = nil
		result = *image
		return nil
	})
	if err != nil {
		return model.Image{}, imageError("finish restore", err)
	}
	return result, nil
}

// FinishRestoreCleanup clears only the matching activated image's trash-cleanup work.
func (r *ImageRepository) FinishRestoreCleanup(ctx context.Context, key, op string) error {
	err := r.withImage(ctx, key, nil, func(tx *gorm.DB, image *model.Image, _ model.User) error {
		if op == "" || image.OperationID != op || image.State != model.ImageStateActive {
			return model.ErrImageBusy
		}
		if image.Operation == "" {
			return nil
		}
		if image.Operation != model.ImageOperationRestoreCleanup {
			return model.ErrImageBusy
		}
		return updateImage(tx, image.ID, map[string]any{"operation": ""})
	})
	return finishImageError("finish restore cleanup", err)
}

// CancelRestore releases a failed restoration reservation after new live objects are removed.
func (r *ImageRepository) CancelRestore(ctx context.Context, key, op string) error {
	err := r.withImage(ctx, key, nil, func(tx *gorm.DB, image *model.Image, _ model.User) error {
		if op == "" || image.OperationID != op || image.State != model.ImageStateTrash {
			return model.ErrImageBusy
		}
		if image.Operation == "" {
			return nil
		}
		if image.Operation != model.ImageOperationRestore {
			return model.ErrImageBusy
		}
		return updateImage(tx, image.ID, map[string]any{"operation": ""})
	})
	return finishImageError("cancel restore", err)
}

// BeginPurge claims idle trash for an owner/admin-authorized physical purge.
func (r *ImageRepository) BeginPurge(ctx context.Context, key, op string, grant model.TokenGrant) (model.Image, error) {
	return r.beginPurge(ctx, key, op, &grant)
}

// BeginSystemPurge is restricted to the trusted host scheduler's dependency interface.
func (r *ImageRepository) BeginSystemPurge(ctx context.Context, key, op string) (model.Image, error) {
	return r.beginPurge(ctx, key, op, nil)
}

func (r *ImageRepository) beginPurge(ctx context.Context, key, op string, grant *model.TokenGrant) (model.Image, error) {
	if err := ctx.Err(); err != nil {
		return model.Image{}, fmt.Errorf("begin purge: %w", err)
	}
	if op == "" {
		return model.Image{}, fmt.Errorf("begin purge: %w", model.ErrInvalidInput)
	}
	var result model.Image
	err := r.withImage(ctx, key, grant, func(tx *gorm.DB, image *model.Image, _ model.User) error {
		if image.State == model.ImageStateTrash && image.Operation == model.ImageOperationPurge && image.OperationID == op {
			result = *image
			return nil
		}
		if image.State != model.ImageStateTrash || image.Operation != "" || image.OperationID == op {
			return model.ErrImageBusy
		}
		if err := updateImage(tx, image.ID, map[string]any{"operation": model.ImageOperationPurge, "operation_id": op}); err != nil {
			return err
		}
		image.Operation = model.ImageOperationPurge
		image.OperationID = op
		result = *image
		return nil
	})
	if err != nil {
		return model.Image{}, imageError("begin purge", err)
	}
	return result, nil
}

// FinishPurge releases metadata and its path only after all physical object versions are removed.
func (r *ImageRepository) FinishPurge(ctx context.Context, key, op string) error {
	err := r.withImage(ctx, key, nil, func(tx *gorm.DB, image *model.Image, _ model.User) error {
		if op == "" || image.OperationID != op || image.State != model.ImageStateTrash ||
			image.Operation != model.ImageOperationPurge {
			return model.ErrImageBusy
		}
		return deleteImage(tx, *image)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, model.ErrNotFound) {
		return nil
	}
	return finishImageError("finish purge", err)
}

// chargeOwner moves an account's used_bytes by delta. The guest anchor (id 0)
// has no counter: guest usage is derived from images.user_id = 0, so its
// transitions only change the image state.
func chargeOwner(tx *gorm.DB, owner model.User, delta int64) error {
	if owner.ID == 0 {
		return nil
	}
	if delta < 0 && -delta > owner.UsedBytes {
		return model.ErrInvalidInput
	}
	return tx.Model(&model.User{}).Where("id = ?", owner.ID).
		Update("used_bytes", gorm.Expr("used_bytes + ?", delta)).Error
}

// checkOwnerQuota applies the account quota, or the guest group's shared quota
// for guest images. Without a guest group there is no guest capacity to
// enforce, so an administrator can still restore earlier guest uploads.
func checkOwnerQuota(ctx context.Context, tx *gorm.DB, owner model.User, additional int64) error {
	if owner.ID != 0 {
		return checkQuota(tx, owner, additional)
	}
	if err := lockGuestQuota(ctx, tx); err != nil {
		return err
	}
	group, ok, err := resolveGuestGroup(ctx, tx)
	if err != nil || !ok {
		return err
	}
	return checkGuestQuota(tx, group, additional)
}
