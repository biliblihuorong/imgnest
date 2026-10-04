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
