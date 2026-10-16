package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// VariantCandidates lists idle active images after the cursor, in ID order,
// whose rule enables responsive variants. Images that already carry variants
// are included so the caller can count them as skipped.
func (r *ImageRepository) VariantCandidates(ctx context.Context, after uint64, limit int) ([]model.Image, error) {
	if limit < 1 || limit > 500 {
		return nil, fmt.Errorf("find variant candidates: %w", model.ErrInvalidInput)
	}
	if err := checkRecordID(ctx, after); err != nil {
		return nil, fmt.Errorf("find variant candidates: %w", err)
	}
	images := []model.Image{}
	err := r.db.WithContext(ctx).
		Where("id > ? AND state = ? AND operation = ?", after, model.ImageStateActive, "").
		Where("policy_id IN (SELECT id FROM policies WHERE responsive_variants = ?)", true).
		Order("id ASC").Limit(limit).Find(&images).Error
	if err != nil {
		return nil, repositoryError("find variant candidates", err)
	}
	return images, nil
}

// BeginVariants journals the variant objects about to be written for an idle
// active image and reserves their bytes against the owner's quota, so the
// usage is counted before any object exists.
func (r *ImageRepository) BeginVariants(ctx context.Context, job model.VariantJob) (model.Image, error) {
	if err := ctx.Err(); err != nil {
		return model.Image{}, fmt.Errorf("begin variants: %w", err)
	}
	if job.OperationID == "" || len(job.Variants) == 0 {
		return model.Image{}, fmt.Errorf("begin variants: %w", model.ErrInvalidInput)
	}
	var result model.Image
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		image, owner, err := lockImageOwner(ctx, tx, job.ImageKey)
		if err != nil {
			return err
		}
		if image.State != model.ImageStateActive || image.Operation != "" || len(image.Variants) > 0 {
			return model.ErrImageBusy
		}
		candidate := image
		candidate.Variants = job.Variants
		declared, err := declaredVariants(candidate)
		if err != nil {
			return err
		}
		objects := make([]model.ObjectReceipt, 0, len(job.Objects))
		var total int64
		for _, object := range job.Objects {
			size, ok := declared[object.Key]
			if !ok || object.Location != model.ObjectLocationCloud || object.OwnerID != image.Key || object.Size != size || object.MIME != model.VariantMIME {
				return model.ErrInvalidInput
			}
			digest, err := objectDigest(object.SHA256)
			if err != nil || digest == "" {
				return model.ErrInvalidInput
			}
			object.SHA256, object.VersionID = digest, ""
			delete(declared, object.Key)
			if object.Size > math.MaxInt64-total {
				return model.ErrQuotaExceeded
			}
			total += object.Size
			objects = append(objects, object)
		}
		if len(declared) != 0 {
			return model.ErrInvalidInput
		}
		for _, receipt := range image.ObjectManifest {
			for _, object := range objects {
				if receipt.Key == object.Key {
					return model.ErrInvalidInput
				}
			}
		}
		if err := checkVariantKeys(tx, image.StorageID, ownVariantKeys(candidate)); err != nil {
			return err
		}
		if err := checkOwnerQuota(ctx, tx, owner, total); err != nil {
			return err
		}
		if image.ChargedBytes > math.MaxInt64-total {
			return model.ErrQuotaExceeded
		}
		if err := chargeOwner(tx, owner, total); err != nil {
			return err
		}
		if err := updateImage(tx, image.ID, map[string]any{"charged_bytes": image.ChargedBytes + total}); err != nil {
			return err
		}
		job.ImageID, job.Objects, job.Bytes = image.ID, objects, total
		if job.CreatedAt.IsZero() {
			job.CreatedAt = time.Now().UTC()
		}
		if err := tx.Create(&job).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return model.ErrImageBusy
			}
			return err
		}
		image.ChargedBytes += total
		result = image
		return nil
	})
	if err != nil {
		return model.Image{}, imageError("begin variants", err)
	}
	return result, nil
}

// RecordVariantReceipt stores the version a variant write produced.
func (r *ImageRepository) RecordVariantReceipt(ctx context.Context, imageID uint64, op string, receipt model.ObjectReceipt) error {
	err := r.withVariantJob(ctx, imageID, op, func(tx *gorm.DB, job *model.VariantJob) error {
		for index, current := range job.Objects {
			if current.Key != receipt.Key || current.Location != receipt.Location {
				continue
			}
			if receipt.Size != current.Size || receipt.OwnerID != current.OwnerID {
				return model.ErrInvalidInput
			}
			job.Objects[index].VersionID = receipt.VersionID
			encoded, err := json.Marshal(job.Objects)
			if err != nil {
				return err
			}
			return tx.Model(&model.VariantJob{}).Where("image_id = ?", job.ImageID).Update("objects", string(encoded)).Error
		}
		return model.ErrInvalidInput
	})
	return finishImageError("record variant receipt", err)
}

// FinishVariants publishes written variants: they join the image's object
// manifest, so every later lifecycle step covers them, and the journal ends.
func (r *ImageRepository) FinishVariants(ctx context.Context, imageID uint64, op string) (model.Image, error) {
	var result model.Image
	err := r.withVariantJob(ctx, imageID, op, func(tx *gorm.DB, job *model.VariantJob) error {
		image, err := lockImageByID(tx, imageID)
		if err != nil {
			return err
		}
		if image.State != model.ImageStateActive || image.Operation != "" || len(image.Variants) > 0 {
			return model.ErrImageBusy
		}
		image.ObjectManifest = append(image.ObjectManifest, job.Objects...)
		image.Variants = job.Variants
		manifest, err := json.Marshal(image.ObjectManifest)
		if err != nil {
			return err
		}
		variants, err := json.Marshal(image.Variants)
		if err != nil {
			return err
		}
		if err := updateImage(tx, image.ID, map[string]any{"object_manifest": string(manifest), "variants": string(variants)}); err != nil {
			return err
		}
		if err := tx.Where("image_id = ?", job.ImageID).Delete(&model.VariantJob{}).Error; err != nil {
			return err
		}
		result = image
		return nil
	})
	if err != nil {
		return model.Image{}, imageError("finish variants", err)
	}
	return result, nil
}

// CancelVariants releases the reserved bytes of a journal whose objects were
// removed. Owner usage only holds the reservation while the image is active;
// a recycled image already released its whole charge.
func (r *ImageRepository) CancelVariants(ctx context.Context, imageID uint64, op string) error {
	err := r.withVariantJob(ctx, imageID, op, func(tx *gorm.DB, job *model.VariantJob) error {
		var snapshot model.Image
		if err := tx.First(&snapshot, "id = ?", imageID).Error; err != nil {
			return err
		}
		image, owner, err := lockImageOwner(ctx, tx, snapshot.Key)
		if err != nil {
			return err
		}
		charged := max(image.ChargedBytes-job.Bytes, 0)
		if image.State == model.ImageStateActive {
			if err := chargeOwner(tx, owner, -(image.ChargedBytes - charged)); err != nil {
				return err
			}
		}
		if err := updateImage(tx, image.ID, map[string]any{"charged_bytes": charged}); err != nil {
			return err
		}
		return tx.Where("image_id = ?", job.ImageID).Delete(&model.VariantJob{}).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, model.ErrNotFound) {
		return nil
	}
	return finishImageError("cancel variants", err)
}

// PendingVariantJobs lists unfinished variant journals for recovery.
func (r *ImageRepository) PendingVariantJobs(ctx context.Context) ([]model.VariantJob, error) {
	jobs := []model.VariantJob{}
	if err := r.db.WithContext(ctx).Order("image_id ASC").Find(&jobs).Error; err != nil {
		return nil, repositoryError("find variant jobs", err)
	}
	return jobs, nil
}

// HasVariantJob reports whether an image has an unfinished variant journal.
func (r *ImageRepository) HasVariantJob(ctx context.Context, imageID uint64) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.VariantJob{}).Where("image_id = ?", imageID).Count(&count).Error; err != nil {
		return false, repositoryError("find variant job", err)
	}
	return count > 0, nil
}

// VariantBackfill reads the historic backfill progress.
func (r *ImageRepository) VariantBackfill(ctx context.Context) (model.VariantBackfill, error) {
	var state model.VariantBackfill
	if err := r.db.WithContext(ctx).First(&state, "id = ?", 1).Error; err != nil {
		return model.VariantBackfill{}, repositoryError("read variant backfill", err)
	}
	return state, nil
}

// SaveVariantBackfill replaces the historic backfill progress.
func (r *ImageRepository) SaveVariantBackfill(ctx context.Context, state model.VariantBackfill) error {
	switch state.Status {
	case model.BackfillIdle, model.BackfillRunning, model.BackfillPaused, model.BackfillDone:
	default:
		return fmt.Errorf("save variant backfill: %w", model.ErrInvalidInput)
	}
	if state.Cursor > math.MaxInt64 {
		return fmt.Errorf("save variant backfill: %w", model.ErrInvalidInput)
	}
	state.ID = 1
	if err := r.db.WithContext(ctx).Save(&state).Error; err != nil {
		return repositoryError("save variant backfill", err)
	}
	return nil
}

func (r *ImageRepository) withVariantJob(ctx context.Context, imageID uint64, op string, apply func(*gorm.DB, *model.VariantJob) error) error {
	if err := checkRecordID(ctx, imageID); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx
		if tx.Name() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var job model.VariantJob
		if err := query.First(&job, "image_id = ?", imageID).Error; err != nil {
			return err
		}
		if op == "" || job.OperationID != op {
			return model.ErrImageBusy
		}
		return apply(tx, &job)
	})
}

// lockImageOwner locks an image's owner (none for guest uploads) before the
// image row, the same order every other image transition uses.
func lockImageOwner(ctx context.Context, tx *gorm.DB, key string) (model.Image, model.User, error) {
	var snapshot model.Image
	if err := tx.First(&snapshot, "key = ?", key).Error; err != nil {
		return model.Image{}, model.User{}, err
	}
	owner := model.User{}
	if snapshot.UserID != 0 {
		user, err := lockUser(ctx, tx, snapshot.UserID)
		if err != nil {
			return model.Image{}, model.User{}, err
		}
		owner = user
	}
	image, err := lockImageByID(tx, snapshot.ID)
	if err != nil {
		return model.Image{}, model.User{}, err
	}
	if image.UserID != owner.ID {
		return model.Image{}, model.User{}, model.ErrImageBusy
	}
	return image, owner, nil
}

func lockImageByID(tx *gorm.DB, id uint64) (model.Image, error) {
	query := tx
	if tx.Name() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var image model.Image
	err := query.First(&image, "id = ?", id).Error
	return image, err
}
