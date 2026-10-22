package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// Update stores management-console changes to a backend's display values,
// switch state, and (already encrypted) configuration, preserving the
// creation timestamp.
func (r *StorageRepository) Update(ctx context.Context, value model.Storage, guard model.StorageUpdateGuard) (model.Storage, error) {
	if err := ctx.Err(); err != nil {
		return model.Storage{}, fmt.Errorf("update storage: %w", err)
	}
	if err := checkRecordID(ctx, value.ID); err != nil {
		return model.Storage{}, fmt.Errorf("update storage: %w", err)
	}
	if strings.TrimSpace(value.Name) == "" || !json.Valid(value.Config) ||
		guard.ExpectedDriver == "" || value.Driver != guard.ExpectedDriver || !json.Valid(guard.ExpectedConfig) {
		return model.Storage{}, fmt.Errorf("update storage: %w", model.ErrInvalidInput)
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, err := lockStorage(ctx, tx, value.ID)
		if err != nil {
			return err
		}
		if existing.Driver != guard.ExpectedDriver || !sameStorageConfig(existing.Config, guard.ExpectedConfig) || existing.PathPrefix != guard.ExpectedPathPrefix {
			return model.ErrImageBusy
		}
		if guard.RequireEmpty {
			var images int64
			if err := tx.Model(&model.Image{}).Where("storage_id = ?", value.ID).Count(&images).Error; err != nil {
				return err
			}
			if images != 0 {
				return model.ErrStillReferenced
			}
		}
		value.CreatedAt = existing.CreatedAt
		return tx.Save(&value).Error
	})
	if err != nil {
		return model.Storage{}, repositoryError("update storage", err)
	}
	return value, nil
}

// lockStorage serializes location changes with authenticated and guest upload
// reservations. SQLite obtains its write lock at BEGIN via _txlock=immediate;
// PostgreSQL requires an explicit row lock for the same guarantee.
func lockStorage(ctx context.Context, tx *gorm.DB, id uint64) (model.Storage, error) {
	query := tx.WithContext(ctx)
	if tx.Name() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var backend model.Storage
	err := query.First(&backend, "id = ?", id).Error
	return backend, err
}

// JSONB may reorder keys and whitespace, so compare configuration values
// instead of serialized bytes. No configuration is included in errors.
func sameStorageConfig(a, b json.RawMessage) bool {
	var first, second any
	decode := func(raw json.RawMessage, target *any) error {
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		return decoder.Decode(target)
	}
	return decode(a, &first) == nil && decode(b, &second) == nil && reflect.DeepEqual(first, second)
}

func validateUploadStorage(ctx context.Context, tx *gorm.DB, backend model.Storage, expected model.Storage) error {
	current, err := lockStorage(ctx, tx, backend.ID)
	if err != nil {
		return err
	}
	if !current.Enabled {
		return model.ErrForbidden
	}
	if expected.ID != current.ID || expected.Driver != current.Driver || expected.PathPrefix != current.PathPrefix || !sameStorageConfig(current.Config, expected.Config) {
		return model.ErrImageBusy
	}
	return nil
}

// Delete removes a backend configuration. Rules still referencing the
// backend are rejected before this runs, and the policies foreign key still
// guards against concurrent creation.
func (r *StorageRepository) Delete(ctx context.Context, id uint64) error {
	if err := checkRecordID(ctx, id); err != nil {
		return fmt.Errorf("delete storage: %w", err)
	}
	deleted := r.db.WithContext(ctx).Delete(&model.Storage{}, "id = ?", id)
	if deleted.Error != nil {
		return repositoryError("delete storage", deleted.Error)
	}
	if deleted.RowsAffected == 0 {
		return fmt.Errorf("delete storage: %w", model.ErrNotFound)
	}
	return nil
}
