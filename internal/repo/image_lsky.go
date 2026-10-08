package repo

import (
	"context"
	"fmt"
	"math"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// ListV1 pages one owner's active images with the Lsky v1 filters: order,
// visibility, album and keyword. album_id zero selects only images that are
// not assigned to any album, matching the Lsky quirk.
func (r *ImageRepository) ListV1(ctx context.Context, ownerID uint64, page, size int, order, permission, keyword string, albumID uint64) ([]model.Image, int64, error) {
	if page < 1 || size < 1 || size > 200 {
		return nil, 0, fmt.Errorf("list v1 images: %w", model.ErrInvalidInput)
	}
	if page-1 > math.MaxInt/size {
		return nil, 0, fmt.Errorf("list v1 images: %w", model.ErrInvalidInput)
	}
	base := func() *gorm.DB {
		query := r.db.WithContext(ctx).Model(&model.Image{}).
			Where("state = ? AND user_id = ?", model.ImageStateActive, ownerID)
		switch permission {
		case "public":
			query = query.Where("is_public = ?", true)
		case "private":
			query = query.Where("is_public = ?", false)
		}
		if albumID == 0 {
			query = query.Where("album_id IS NULL")
		} else {
			query = query.Where("album_id = ?", albumID)
		}
		if keyword != "" {
			pattern := "%" + escapeLike(keyword) + "%"
			query = query.Where(
				"(LOWER(origin_name) LIKE LOWER(?) ESCAPE '\\' OR LOWER(path || '.' || ext) LIKE LOWER(?) ESCAPE '\\')",
				pattern, pattern,
			)
		}
		return query
	}
	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, repositoryError("count v1 images", err)
	}
	images := []model.Image{}
	if err := base().
		Order(v1ImageOrder(order)).
		Limit(size).Offset((page - 1) * size).
		Find(&images).Error; err != nil {
		return nil, 0, repositoryError("list v1 images", err)
	}
	return images, total, nil
}

func v1ImageOrder(order string) string {
	switch order {
	case "earliest":
		return "created_at ASC, id ASC"
	case "utmost":
		return "size DESC, id DESC"
	case "least":
		return "size ASC, id ASC"
	default:
		return "created_at DESC, id DESC"
	}
}
