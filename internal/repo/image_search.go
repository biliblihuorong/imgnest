package repo

import (
	"context"
	"fmt"
	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
	"time"
)

func (r *ImageRepository) listSearch(ctx context.Context, filter model.ImageListFilter, page, size int) ([]model.Image, int64, error) {
	if page < 1 || page > 10000 || (size != 20 && size != 50 && size != 100) || filter.Admin || filter.Trash || filter.UserID == 0 {
		return nil, 0, model.ErrInvalidInput
	}
	q := filter.Search
	if q.MinBytes != nil && (*q.MinBytes < 0 || *q.MinBytes > 1024000000000) || q.MaxBytes != nil && (*q.MaxBytes < 0 || *q.MaxBytes > 1024000000000) {
		return nil, 0, model.ErrInvalidInput
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// COUNT and rows share exactly this constructor, including the locked route scope.
	base := func() *gorm.DB {
		db := r.db.WithContext(ctx).Model(&model.Image{}).Where("images.user_id = ? AND images.state = ?", filter.UserID, model.ImageStateActive)
		if filter.AlbumID != nil {
			db = db.Where("images.album_id = ?", *filter.AlbumID)
		}
		for _, term := range q.Terms {
			db = literalContains(db, r.db.Name(), "images.filename_search", term)
		}
		if len(q.Formats) > 0 {
			db = db.Where("("+trustedFormatSQL+") IN ?", q.Formats)
		}
		if len(q.AlbumIDs) > 0 && q.IncludeUnfiled {
			db = db.Where("(images.album_id IN ? OR images.album_id IS NULL OR images.album_id = 0)", q.AlbumIDs)
		} else if len(q.AlbumIDs) > 0 {
			db = db.Where("images.album_id IN ?", q.AlbumIDs)
		} else if q.IncludeUnfiled {
			db = db.Where("(images.album_id IS NULL OR images.album_id = 0)")
		}
		if q.Camera != nil {
			predicate := "instr(image_exif.camera_search, ?) > 0 OR instr(image_exif.lens_search, ?) > 0"
			if r.db.Name() == "postgres" {
				predicate = "strpos(image_exif.camera_search, ?) > 0 OR strpos(image_exif.lens_search, ?) > 0"
			}
			db = db.Where("EXISTS (SELECT 1 FROM image_exif WHERE image_exif.image_id = images.id AND ("+predicate+"))", *q.Camera, *q.Camera)
		}
		if q.MinBytes != nil {
			db = db.Where("images.size >= ?", *q.MinBytes)
		}
		if q.MaxBytes != nil {
			db = db.Where("images.size <= ?", *q.MaxBytes)
		}
		if q.AfterUTC != nil {
			db = db.Where("images.created_at >= ?", q.AfterUTC.UTC())
		}
		if q.BeforeUTC != nil {
			db = db.Where("images.created_at < ?", q.BeforeUTC.UTC())
		}
		switch q.Visibility {
		case "public":
			db = db.Where("images.is_public = ?", true)
		case "private":
			db = db.Where("images.is_public = ?", false)
		}
		return db
	}
	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, repositoryError("count search images", err)
	}
	images := []model.Image{}
	if err := base().Order(searchOrder(q.Sort, r.db.Name())).Limit(size).Offset((page - 1) * size).Find(&images).Error; err != nil {
		return nil, 0, repositoryError("search images", err)
	}
	return images, total, nil
}

// MIME was produced by trusted upload decoding/conversion, never a filename.
const trustedFormatSQL = `CASE images.mime WHEN 'image/jpeg' THEN 'jpg' WHEN 'image/png' THEN 'png' WHEN 'image/gif' THEN 'gif' WHEN 'image/webp' THEN 'webp' WHEN 'image/avif' THEN 'avif' WHEN 'image/bmp' THEN 'bmp' WHEN 'image/tiff' THEN 'tiff' WHEN 'image/svg+xml' THEN 'svg' WHEN 'image/heic' THEN 'heic' WHEN 'image/heif' THEN 'heif' ELSE 'unknown' END`

// Both engines provide case-sensitive literal substring functions; no LIKE
// wildcard, collation-based case folding, or escaping ambiguity is involved.
func literalContains(db *gorm.DB, driver, column, value string) *gorm.DB {
	if driver == "postgres" {
		return db.Where("strpos("+column+", ?) > 0", value)
	}
	return db.Where("instr("+column+", ?) > 0", value)
}
func binaryColumn(driver, column string) string {
	if driver == "postgres" {
		return column + ` COLLATE "C"`
	}
	return column + " COLLATE BINARY"
}
func searchOrder(value, driver string) string {
	switch value {
	case "oldest":
		return "images.created_at ASC, images.id ASC"
	case "size-desc":
		return "images.size IS NULL ASC, images.size DESC, images.id DESC"
	case "size-asc":
		return "images.size IS NULL ASC, images.size ASC, images.id ASC"
	case "name-asc":
		return "images.filename_search IS NULL ASC, " + binaryColumn(driver, "images.filename_search") + " ASC, images.id ASC"
	case "name-desc":
		return "images.filename_search IS NULL ASC, " + binaryColumn(driver, "images.filename_search") + " DESC, images.id DESC"
	default:
		return "images.created_at DESC, images.id DESC"
	}
}

// SearchAlbums finds only owned candidates. Its projection never reads counts,
// covers, image names, or EXIF, including during a failed resolution.
func (r *AlbumRepository) SearchAlbums(ctx context.Context, ownerID uint64, id *uint64, name *string, scope *uint64) ([]model.Album, error) {
	db := r.db.WithContext(ctx).Model(&model.Album{}).Select("id, name").Where("user_id = ?", ownerID)
	if scope != nil {
		db = db.Where("id = ?", *scope)
	}
	if id != nil {
		db = db.Where("id = ?", *id)
	} else if name != nil {
		db = db.Where(binaryColumn(r.db.Name(), "name_nfc")+" = ?", *name)
	} else {
		return nil, model.ErrInvalidInput
	}
	albums := []model.Album{}
	if err := db.Order("id ASC").Limit(6).Find(&albums).Error; err != nil {
		return nil, repositoryError("resolve search album", err)
	}
	return albums, nil
}

// SuggestAlbums fetches one extra row for hasMore without touching images.
func (r *AlbumRepository) SuggestAlbums(ctx context.Context, ownerID uint64, keyword string, page, size int, scope *uint64) ([]model.Album, bool, error) {
	if page < 1 || page > 10000 || size < 1 || size > 20 {
		return nil, false, fmt.Errorf("suggest albums: %w", model.ErrInvalidInput)
	}
	db := r.db.WithContext(ctx).Model(&model.Album{}).Select("id, name").Where("user_id = ?", ownerID)
	if scope != nil {
		db = db.Where("id = ?", *scope)
	}
	if keyword != "" {
		db = literalContains(db, r.db.Name(), "name_search", keyword)
	}
	albums := []model.Album{}
	if err := db.Order(binaryColumn(r.db.Name(), "name_search") + " ASC, id ASC").Limit(size + 1).Offset((page - 1) * size).Find(&albums).Error; err != nil {
		return nil, false, repositoryError("suggest albums", err)
	}
	more := len(albums) > size
	if more {
		albums = albums[:size]
	}
	return albums, more, nil
}
