package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// SettingsRepository reads site settings needed by authentication services.
type SettingsRepository struct{ db *gorm.DB }

// NewSettingsRepository creates a site-settings persistence adapter.
func NewSettingsRepository(ctx context.Context, db *gorm.DB) (*SettingsRepository, error) {
	if err := checkDatabase(ctx, db); err != nil {
		return nil, err
	}
	return &SettingsRepository{db: db}, nil
}

// RegistrationEnabled returns the site's explicit registration switch.
func (r *SettingsRepository) RegistrationEnabled(ctx context.Context) (bool, error) {
	setting, err := r.find(ctx, "registration_enabled")
	if err != nil {
		return false, err
	}
	var enabled *bool
	if err := json.Unmarshal(setting.Value, &enabled); err != nil {
		return false, databaseError("decode registration setting", err)
	}
	if enabled == nil {
		return false, errors.New("registration setting must be a boolean")
	}
	return *enabled, nil
}

// DefaultGroupID returns the configured, existing default account group.
func (r *SettingsRepository) DefaultGroupID(ctx context.Context) (uint64, error) {
	setting, err := r.find(ctx, "default_group_id")
	if err != nil {
		return 0, err
	}
	var id uint64
	if err := json.Unmarshal(setting.Value, &id); err != nil {
		return 0, databaseError("decode default group setting", err)
	}
	if id == 0 {
		return 0, errors.New("default group must be a positive ID")
	}
	var group model.Group
	if err := r.db.WithContext(ctx).First(&group, "id = ?", id).Error; err != nil {
		return 0, repositoryError("find default group", err)
	}
	return id, nil
}

// TrashDays returns the bounded, nonnegative retention period for image lifecycle operations.
func (r *SettingsRepository) TrashDays(ctx context.Context) (int, error) {
	setting, err := r.find(ctx, "trash_days")
	if err != nil {
		return 0, err
	}
	var days *int64
	if err := json.Unmarshal(setting.Value, &days); err != nil {
		return 0, fmt.Errorf("decode trash retention: %w", model.ErrInvalidInput)
	}
	if days == nil {
		return 0, fmt.Errorf("decode trash retention: %w", model.ErrInvalidInput)
	}
	if *days < 0 || *days > 36500 {
		return 0, fmt.Errorf("decode trash retention: %w", model.ErrInvalidInput)
	}
	return int(*days), nil
}

// SiteName returns the configured public site name, or an empty string when
// no name was ever stored; the service layer owns the display fallback.
func (r *SettingsRepository) SiteName(ctx context.Context) (string, error) {
	setting, found, err := r.findOptional(ctx, "site_name")
	if err != nil || !found {
		return "", err
	}
	var name *string
	if err := json.Unmarshal(setting.Value, &name); err != nil {
		return "", databaseError("decode site name", err)
	}
	if name == nil {
		return "", nil
	}
	return *name, nil
}

// GalleryEnabled returns the public gallery switch. An absent key reads as
// closed so deployments without the seed never leak a gallery.
func (r *SettingsRepository) GalleryEnabled(ctx context.Context) (bool, error) {
	setting, found, err := r.findOptional(ctx, "gallery_enabled")
	if err != nil || !found {
		return false, err
	}
	var enabled *bool
	if err := json.Unmarshal(setting.Value, &enabled); err != nil {
		return false, databaseError("decode gallery setting", err)
	}
	if enabled == nil {
		return false, nil
	}
	return *enabled, nil
}

// AvatarConfig returns the site-wide avatar provider with its configuration
// version (the settings row's update time as Unix seconds). A missing key or
// an unsupported stored value reads as the default provider with version 0 so
// a corrupt setting degrades to the known default instead of failing every
// authenticated request; unsupported providers are never contacted.
func (r *SettingsRepository) AvatarConfig(ctx context.Context) (model.AvatarConfig, error) {
	setting, found, err := r.findOptional(ctx, "avatar_provider")
	if err != nil || !found {
		return model.AvatarConfig{Provider: model.DefaultAvatarProvider}, err
	}
	var provider *string
	if err := json.Unmarshal(setting.Value, &provider); err != nil {
		return model.AvatarConfig{Provider: model.DefaultAvatarProvider}, nil
	}
	if !model.ValidAvatarProvider(deref(provider)) {
		return model.AvatarConfig{Provider: model.DefaultAvatarProvider}, nil
	}
	return model.AvatarConfig{Provider: deref(provider), Version: setting.UpdatedAt.Unix()}, nil
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// ReadSettings returns every stored setting keyed by its settings key. Callers
// apply their own defaults for absent keys.
func (r *SettingsRepository) ReadSettings(ctx context.Context) (map[string]json.RawMessage, error) {
	var settings []model.Setting
	if err := r.db.WithContext(ctx).Find(&settings).Error; err != nil {
		return nil, databaseError("list settings", err)
	}
	values := make(map[string]json.RawMessage, len(settings))
	for _, setting := range settings {
		values[setting.Key] = setting.Value
	}
	return values, nil
}

// UpdateSettings upserts the given JSON values one key at a time inside a
// single transaction; settings keys not present stay untouched.
func (r *SettingsRepository) UpdateSettings(ctx context.Context, values map[string]json.RawMessage) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for key, value := range values {
			if !json.Valid(value) {
				return fmt.Errorf("update setting %s: %w", key, model.ErrInvalidInput)
			}
			if tx.Name() == "postgres" {
				err := tx.Exec(
					"INSERT INTO settings (key, value) VALUES (?, ?::jsonb) "+
						"ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = CURRENT_TIMESTAMP",
					key, string(value),
				).Error
				if err != nil {
					return err
				}
				continue
			}
			err := tx.Exec(
				"INSERT INTO settings (key, value) VALUES (?, ?) "+
					"ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP",
				key, string(value),
			).Error
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return databaseError("update settings", err)
	}
	return nil
}

func (r *SettingsRepository) findOptional(ctx context.Context, key string) (model.Setting, bool, error) {
	var setting model.Setting
	err := r.db.WithContext(ctx).First(&setting, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Setting{}, false, nil
	}
	if err != nil {
		return model.Setting{}, false, databaseError("find setting", err)
	}
	return setting, true, nil
}

func (r *SettingsRepository) find(ctx context.Context, key string) (model.Setting, error) {
	var setting model.Setting
	if err := r.db.WithContext(ctx).First(&setting, "key = ?", key).Error; err != nil {
		return model.Setting{}, repositoryError("find setting", err)
	}
	return setting, nil
}

func checkDatabase(ctx context.Context, db *gorm.DB) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("construct repository: %w", err)
	}
	if db == nil {
		return fmt.Errorf("construct repository: %w", model.ErrInvalidInput)
	}
	return nil
}

func repositoryError(operation string, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%s: %w", operation, model.ErrNotFound)
	}
	return databaseError(operation, err)
}
