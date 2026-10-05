package repo

import (
	"context"
	"fmt"
	"math"
	"slices"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// OwnedActiveImage resolves an image that must exist, belong to the owner and
// still be active. Missing images report not-found; every other mismatch
// reports invalid input so cover selectors learn nothing about other accounts.
func (r *ImageRepository) OwnedActiveImage(ctx context.Context, ownerID, imageID uint64) (model.Image, error) {
	if err := checkRecordID(ctx, imageID); err != nil {
		return model.Image{}, fmt.Errorf("find own active image: %w", err)
	}
	image, err := r.FindByID(ctx, imageID)
	if err != nil {
		return model.Image{}, fmt.Errorf("find own active image: %w", err)
	}
	if image.UserID != ownerID || image.State != model.ImageStateActive {
		return model.Image{}, fmt.Errorf("find own active image: %w", model.ErrInvalidInput)
	}
	return image, nil
}

// SetAlbum reassigns one owned active image inside the caller-proofed image
// transaction. Moving out writes SQL NULL so the v1 unassigned quirk and the
// native unassigned filter agree on the same rows.
func (r *ImageRepository) SetAlbum(ctx context.Context, key string, albumID uint64, grant model.TokenGrant) error {
	err := r.withImage(ctx, key, &grant, func(tx *gorm.DB, image *model.Image, _ model.User) error {
		if image.State != model.ImageStateActive {
			return model.ErrInvalidInput
		}
		albumIDs := []uint64{}
		for _, id := range []uint64{image.AlbumID, albumID} {
			if id != 0 && !slices.Contains(albumIDs, id) {
				albumIDs = append(albumIDs, id)
			}
		}
		slices.Sort(albumIDs)
		if err := lockImageAlbums(tx, image.UserID, albumIDs); err != nil {
			return err
		}
		values := map[string]any{"album_id": nil}
		if albumID > 0 {
			values["album_id"] = albumID
		}
		if err := updateImage(tx, image.ID, values); err != nil {
			return err
		}
		for _, id := range albumIDs {
			if err := refreshAlbumCount(tx, id, 0); err != nil {
				return err
			}
		}
		return nil
	})
	return finishImageError("set image album", err)
}

// lockImageAlbums follows withImage's owner/image locks. Callers pass distinct
// ascending IDs so concurrent moves cannot invert the album lock order.
func lockImageAlbums(tx *gorm.DB, ownerID uint64, albumIDs []uint64) error {
	for _, id := range albumIDs {
		query := tx
		if tx.Name() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var album model.Album
		if err := query.First(&album, "id = ?", id).Error; err != nil {
			return err
		}
		if album.UserID != ownerID {
			return model.ErrForbidden
		}
	}
	return nil
}

// refreshAlbumCount repairs pre-existing drift as well as the current change.
// The album is locked before counting. Lifecycle callers use a pending delta
// before their image state transition; SetAlbum calls after moving membership.
func refreshAlbumCount(tx *gorm.DB, albumID uint64, delta int64) error {
	var active int64
	if err := tx.Model(&model.Image{}).
		Where("album_id = ? AND state = ?", albumID, model.ImageStateActive).
		Count(&active).Error; err != nil {
		return err
	}
	return tx.Model(&model.Album{}).Where("id = ?", albumID).
		Update("image_count", active+delta).Error
}

// ListGallery pages public active images newest first together with their
// uploader's username. Guest uploads carry no account row and the inner join
// keeps them out of the public gallery.
func (r *ImageRepository) ListGallery(ctx context.Context, page, size int) ([]model.GalleryImage, int64, error) {
	if page < 1 || size < 1 || size > 200 {
		return nil, 0, fmt.Errorf("list gallery: %w", model.ErrInvalidInput)
	}
	if page-1 > math.MaxInt/size {
		return nil, 0, fmt.Errorf("list gallery: %w", model.ErrInvalidInput)
	}
	base := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&model.Image{}).
			Where("images.state = ? AND images.is_public = ?", model.ImageStateActive, true).
			Joins("JOIN users ON users.id = images.user_id")
	}
	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, repositoryError("count gallery", err)
	}
	rows := []model.GalleryImage{}
	if err := base().
		Select("images.*, users.username AS uploader").
		Order("images.created_at DESC, images.id DESC").
		Limit(size).Offset((page - 1) * size).
		Scan(&rows).Error; err != nil {
		return nil, 0, repositoryError("list gallery", err)
	}
	return rows, total, nil
}
