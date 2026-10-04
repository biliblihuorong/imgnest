package repo

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

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
		Select("albums.id, albums.user_id, albums.name, albums.intro, albums.is_public, albums.cover_image_id, COUNT(images.id) AS image_count").
		Joins("LEFT JOIN images ON images.album_id = albums.id AND images.state = ?", model.ImageStateActive).
		Group("albums.id").
		Order(albumOrder(order)).
		Limit(size).Offset((page - 1) * size).
		Find(&albums).Error; err != nil {
		return nil, 0, repositoryError("list albums", err)
	}
	return albums, total, nil
}

// FindOwned resolves one album owned by the caller.
func (r *AlbumRepository) FindOwned(ctx context.Context, ownerID, albumID uint64) (model.Album, error) {
	if err := checkRecordID(ctx, albumID); err != nil {
		return model.Album{}, fmt.Errorf("find album: %w", err)
	}
	var album model.Album
	if err := r.db.WithContext(ctx).First(&album, "id = ? AND user_id = ?", albumID, ownerID).Error; err != nil {
		return model.Album{}, repositoryError("find album", err)
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
