package repo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const mailSettingKey = "mail_config"

// ReadMailConfig returns the stored SMTP configuration, or nil when none was saved.
func (r *SettingsRepository) ReadMailConfig(ctx context.Context) (json.RawMessage, error) {
	value, found, err := r.findOptional(ctx, mailSettingKey)
	if err != nil || !found {
		return nil, err
	}
	return value.Value, nil
}

// WriteMailConfig replaces the SMTP configuration, including its encrypted password.
func (r *SettingsRepository) WriteMailConfig(ctx context.Context, value json.RawMessage) error {
	if !json.Valid(value) || len(value) > 16<<10 {
		return model.ErrInvalidInput
	}
	// Keep the sealed password out of any operator-enabled SQL log.
	db := r.db.WithContext(ctx).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	cast := "?"
	if db.Name() == "postgres" {
		cast = "?::jsonb"
	}
	err := db.Exec("INSERT INTO settings (key, value) VALUES (?, "+cast+") "+
		"ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP",
		mailSettingKey, string(value)).Error
	if err != nil {
		return fmt.Errorf("write mail settings: %w", model.ErrStorage)
	}
	return nil
}

// EmailVerificationEnabled returns the registration email-code switch; an
// absent key reads as off.
func (r *SettingsRepository) EmailVerificationEnabled(ctx context.Context) (bool, error) {
	setting, found, err := r.findOptional(ctx, "email_verification_enabled")
	if err != nil || !found {
		return false, err
	}
	var enabled *bool
	if err := json.Unmarshal(setting.Value, &enabled); err != nil {
		return false, databaseError("decode email verification setting", err)
	}
	return enabled != nil && *enabled, nil
}
