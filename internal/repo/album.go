package repo

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// AlbumRepository pages one owner's albums with live image counts.
type AlbumRepository struct{ db *gorm.DB }

// NewAlbumRepository constructs the album persistence adapter.
func NewAlbumRepository(ctx context.Context, db *gorm.DB) (*AlbumRepository, error) {
	if err := checkDatabase(ctx, db); err != nil {
		return nil, err
	}
	return &AlbumRepository{db: db}, nil
}

// ListByOwner pages one owner's albums. image_count always reflects the live
// number of active images so the value survives restarts and imports. The
// column list is explicit because albums already owns a stored image_count;
// a duplicated output name would break PostgreSQL (SQLite tolerates it).
func (r *AlbumRepository) ListByOwner(ctx context.Context, ownerID uint64, page, size int, order, keyword string) ([]model.Album, int64, error) {
	if page < 1 || size < 1 || size > 200 {
		return nil, 0, fmt.Errorf("list albums: %w", model.ErrInvalidInput)
	}
	if page-1 > math.MaxInt/size {
		return nil, 0, fmt.Errorf("list albums: %w", model.ErrInvalidInput)
	}
	base := func() *gorm.DB {
		query := r.db.WithContext(ctx).Table("albums").Where("albums.user_id = ?", ownerID)
		if keyword != "" {
			pattern := "%" + escapeLike(keyword) + "%"
			query = query.Where("(albums.name LIKE ? ESCAPE '\\' OR albums.intro LIKE ? ESCAPE '\\')", pattern, pattern)
		}
		return query
	}
	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, repositoryError("count albums", err)
	}
	albums := []model.Album{}
	if err := base().
		Select(albumColumns+", COUNT(images.id) AS image_count").
		Joins("LEFT JOIN images ON images.album_id = albums.id AND images.state = ?", model.ImageStateActive).
		Group("albums.id").
		Order(albumOrder(order)).
		Limit(size).Offset((page - 1) * size).
		Find(&albums).Error; err != nil {
		return nil, 0, repositoryError("list albums", err)
	}
	return albums, total, nil
}

// albumColumns is the explicit column list shared by every album read. It is
// explicit because albums already owns a stored image_count and a duplicated
// output name would break PostgreSQL (SQLite tolerates it).
const albumColumns = "albums.id, albums.user_id, albums.name, albums.intro, albums.is_public, albums.cover_image_id, albums.created_at, albums.updated_at"

// Create persists one owner's album row with its live zero count.
func (r *AlbumRepository) Create(ctx context.Context, album model.Album) (model.Album, error) {
	if err := ctx.Err(); err != nil {
		return model.Album{}, fmt.Errorf("create album: %w", err)
	}
	if strings.TrimSpace(album.Name) == "" {
		return model.Album{}, fmt.Errorf("create album: %w", model.ErrInvalidInput)
	}
	if err := r.db.WithContext(ctx).Create(&album).Error; err != nil {
		return model.Album{}, repositoryError("create album", err)
	}
	return album, nil
}

// Update applies validated field changes to one owned album inside a
// transaction and returns the reloaded row with its live image count.
func (r *AlbumRepository) Update(ctx context.Context, ownerID, albumID uint64, values map[string]any) (model.Album, error) {
	if err := checkRecordID(ctx, albumID); err != nil {
		return model.Album{}, fmt.Errorf("update album: %w", err)
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var album model.Album
		if err := tx.First(&album, "id = ?", albumID).Error; err != nil {
			return err
		}
		if album.UserID != ownerID {
			return model.ErrForbidden
		}
		changes := make(map[string]any, len(values)+1)
		for key, value := range values {
			changes[key] = value
		}
		changes["updated_at"] = time.Now().UTC()
		return tx.Model(&model.Album{}).Where("id = ?", albumID).Updates(changes).Error
	})
	if errors.Is(err, model.ErrForbidden) {
		return model.Album{}, fmt.Errorf("update album: %w", model.ErrForbidden)
	}
	if err != nil {
		return model.Album{}, repositoryError("update album", err)
	}
	return r.FindOwned(ctx, ownerID, albumID)
}

// FindOwned resolves one album owned by the caller with its live image count.
// An existing album owned by somebody else reports forbidden so callers can
// mirror the image service's foreign-resource behavior.
func (r *AlbumRepository) FindOwned(ctx context.Context, ownerID, albumID uint64) (model.Album, error) {
	if err := checkRecordID(ctx, albumID); err != nil {
		return model.Album{}, fmt.Errorf("find album: %w", err)
	}
	var album model.Album
	if err := r.db.WithContext(ctx).Table("albums").
		Select(albumColumns+", COUNT(images.id) AS image_count").
		Joins("LEFT JOIN images ON images.album_id = albums.id AND images.state = ?", model.ImageStateActive).
		Where("albums.id = ?", albumID).
		Group("albums.id").
		First(&album).Error; err != nil {
		return model.Album{}, repositoryError("find album", err)
	}
	if album.UserID != ownerID {
		return model.Album{}, fmt.Errorf("find album: %w", model.ErrForbidden)
	}
	return album, nil
}

// CountByOwner counts one owner's albums for the profile response.
func (r *AlbumRepository) CountByOwner(ctx context.Context, ownerID uint64) (int64, error) {
	if err := checkRecordID(ctx, ownerID); err != nil {
		return 0, fmt.Errorf("count owner albums: %w", err)
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.Album{}).
		Where("user_id = ?", ownerID).
		Count(&count).Error; err != nil {
		return 0, repositoryError("count owner albums", err)
	}
	return count, nil
}

// DeleteOwned removes one owned album inside a transaction and detaches its
// images by clearing their album reference; the images themselves stay.
func (r *AlbumRepository) DeleteOwned(ctx context.Context, ownerID, albumID uint64) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var album model.Album
		if err := tx.First(&album, "id = ?", albumID).Error; err != nil {
			return err
		}
		if album.UserID != ownerID {
			return model.ErrForbidden
		}
		if err := tx.Model(&model.Image{}).
			Where("album_id = ? AND user_id = ?", albumID, ownerID).
			Update("album_id", nil).Error; err != nil {
			return err
		}
		return tx.Delete(&model.Album{}, "id = ?", albumID).Error
	})
	if errors.Is(err, model.ErrForbidden) {
		return fmt.Errorf("delete album: %w", model.ErrForbidden)
	}
	if err != nil {
		return repositoryError("delete album", err)
	}
	return nil
}

func albumOrder(order string) string {
	switch order {
	case "earliest":
		return "albums.id ASC"
	case "most":
		return "image_count DESC, albums.id ASC"
	case "least":
		return "image_count ASC, albums.id ASC"
	default:
		return "albums.id DESC"
	}
}

func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}
