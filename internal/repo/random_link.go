package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// RandomLinkRepository persists per-album random-image links and reads the
// redirect candidates they serve.
type RandomLinkRepository struct{ db *gorm.DB }

// NewRandomLinkRepository constructs the random-link persistence adapter.
func NewRandomLinkRepository(ctx context.Context, db *gorm.DB) (*RandomLinkRepository, error) {
	if err := checkDatabase(ctx, db); err != nil {
		return nil, err
	}
	return &RandomLinkRepository{db: db}, nil
}

// FindByAlbum reports model.ErrNotFound when the album has no link.
func (r *RandomLinkRepository) FindByAlbum(ctx context.Context, albumID uint64) (model.RandomLink, error) {
	if err := checkRecordID(ctx, albumID); err != nil {
		return model.RandomLink{}, fmt.Errorf("find random link: %w", err)
	}
	var link model.RandomLink
	if err := r.db.WithContext(ctx).First(&link, "album_id = ?", albumID).Error; err != nil {
		return model.RandomLink{}, repositoryError("find random link", err)
	}
	return link, nil
}

// Create reports album_id or token unique violations as model.ErrRandomLinkExists.
func (r *RandomLinkRepository) Create(ctx context.Context, link model.RandomLink) (model.RandomLink, error) {
	if err := ctx.Err(); err != nil {
		return model.RandomLink{}, fmt.Errorf("create random link: %w", err)
	}
	if link.UserID == 0 || link.AlbumID == 0 || link.Token == "" {
		return model.RandomLink{}, fmt.Errorf("create random link: %w", model.ErrInvalidInput)
	}
	if err := r.db.WithContext(ctx).Create(&link).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return model.RandomLink{}, fmt.Errorf("create random link: %w", model.ErrRandomLinkExists)
		}
		return model.RandomLink{}, repositoryError("create random link", err)
	}
	return link, nil
}

// Update changes only "enabled" and/or "token"; other keys are ErrInvalidInput.
func (r *RandomLinkRepository) Update(ctx context.Context, albumID uint64, values map[string]any) (model.RandomLink, error) {
	if err := checkRecordID(ctx, albumID); err != nil {
		return model.RandomLink{}, fmt.Errorf("update random link: %w", err)
	}
	changes := make(map[string]any, len(values)+1)
	for key, value := range values {
		if key != "enabled" && key != "token" {
			return model.RandomLink{}, fmt.Errorf("update random link: %w", model.ErrInvalidInput)
		}
		changes[key] = value
	}
	if len(changes) == 0 {
		return model.RandomLink{}, fmt.Errorf("update random link: %w", model.ErrInvalidInput)
	}
	changes["updated_at"] = time.Now().UTC()
	result := r.db.WithContext(ctx).Model(&model.RandomLink{}).Where("album_id = ?", albumID).Updates(changes)
	if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
		return model.RandomLink{}, fmt.Errorf("update random link: %w", model.ErrRandomLinkExists)
	}
	if result.Error != nil {
		return model.RandomLink{}, repositoryError("update random link", result.Error)
	}
	if result.RowsAffected == 0 {
		return model.RandomLink{}, fmt.Errorf("update random link: %w", model.ErrNotFound)
	}
	return r.FindByAlbum(ctx, albumID)
}

// DeleteByAlbum is idempotent.
func (r *RandomLinkRepository) DeleteByAlbum(ctx context.Context, albumID uint64) error {
	if err := checkRecordID(ctx, albumID); err != nil {
		return fmt.Errorf("delete random link: %w", err)
	}
	if err := r.db.WithContext(ctx).Delete(&model.RandomLink{}, "album_id = ?", albumID).Error; err != nil {
		return repositoryError("delete random link", err)
	}
	return nil
}

// FindByToken returns the link with its owner row.
func (r *RandomLinkRepository) FindByToken(ctx context.Context, token string) (model.RandomLink, model.User, error) {
	if err := ctx.Err(); err != nil {
		return model.RandomLink{}, model.User{}, fmt.Errorf("find random link by token: %w", err)
	}
	var link model.RandomLink
	if err := r.db.WithContext(ctx).First(&link, "token = ?", token).Error; err != nil {
		return model.RandomLink{}, model.User{}, repositoryError("find random link by token", err)
	}
	var owner model.User
	if err := r.db.WithContext(ctx).First(&owner, "id = ?", link.UserID).Error; err != nil {
		return model.RandomLink{}, model.User{}, repositoryError("find random link owner", err)
	}
	return link, owner, nil
}

// Candidates returns up to limit usable active images in random order, so an
// album larger than the limit contributes a fresh sample on every refill.
func (r *RandomLinkRepository) Candidates(ctx context.Context, albumID uint64, limit int) ([]model.RandomCandidate, error) {
	if err := checkRecordID(ctx, albumID); err != nil {
		return nil, fmt.Errorf("list random candidates: %w", err)
	}
	if limit < 1 {
		return nil, fmt.Errorf("list random candidates: %w", model.ErrInvalidInput)
	}
	candidates := []model.RandomCandidate{}
	if err := r.db.WithContext(ctx).Model(&model.Image{}).
		Select("storage_id, path, ext, has_webp, has_original").
		Where("album_id = ? AND state = ? AND (has_original = ? OR has_webp = ?)", albumID, model.ImageStateActive, true, true).
		Order("RANDOM()").
		Limit(limit).
		Find(&candidates).Error; err != nil {
		return nil, repositoryError("list random candidates", err)
	}
	return candidates, nil
}
