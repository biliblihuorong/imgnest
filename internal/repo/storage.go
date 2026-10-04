package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// StorageRepository persists backend configuration without exposing its secrets.
type StorageRepository struct{ db *gorm.DB }

// NewStorageRepository constructs the runtime backend configuration adapter.
func NewStorageRepository(ctx context.Context, db *gorm.DB) (*StorageRepository, error) {
	if err := checkDatabase(ctx, db); err != nil {
		return nil, err
	}
	return &StorageRepository{db: db}, nil
}

// Create stores a new backend configuration prepared by the trusted configuration service.
func (r *StorageRepository) Create(ctx context.Context, value model.Storage) (model.Storage, error) {
	if err := ctx.Err(); err != nil {
		return model.Storage{}, fmt.Errorf("create storage: %w", err)
	}
	validDriver := value.Driver == "local" || value.Driver == "s3"
	if strings.TrimSpace(value.Name) == "" || !validDriver || !json.Valid(value.Config) {
		return model.Storage{}, fmt.Errorf("create storage: %w", model.ErrInvalidInput)
	}
	if err := r.db.WithContext(ctx).Create(&value).Error; err != nil {
		return model.Storage{}, repositoryError("create storage", err)
	}
	return value, nil
}

// Find returns a backend configuration, including disabled backends required for cleanup.
func (r *StorageRepository) Find(ctx context.Context, id uint64) (model.Storage, error) {
	if err := checkRecordID(ctx, id); err != nil {
		return model.Storage{}, fmt.Errorf("find storage: %w", err)
	}
	var value model.Storage
	if err := r.db.WithContext(ctx).First(&value, "id = ?", id).Error; err != nil {
		return model.Storage{}, repositoryError("find storage", err)
	}
	return value, nil
}

// List returns configured backends in database ID order.
func (r *StorageRepository) List(ctx context.Context) ([]model.Storage, error) {
	values := []model.Storage{}
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&values).Error; err != nil {
		return nil, repositoryError("list storages", err)
	}
	return values, nil
}

// SetBaseURL changes the access prefix without changing backend object ownership.
func (r *StorageRepository) SetBaseURL(ctx context.Context, id uint64, baseURL string) (model.Storage, error) {
	if err := checkRecordID(ctx, id); err != nil {
		return model.Storage{}, fmt.Errorf("set storage URL: %w", err)
	}
	updated := r.db.WithContext(ctx).Model(&model.Storage{}).Where("id = ?", id).Update("base_url", baseURL)
	if updated.Error != nil {
		return model.Storage{}, repositoryError("set storage URL", updated.Error)
	}
	if updated.RowsAffected == 0 {
		return model.Storage{}, fmt.Errorf("set storage URL: %w", model.ErrNotFound)
	}
	return r.Find(ctx, id)
}
