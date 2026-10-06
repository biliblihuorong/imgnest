package repo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const captchaSettingKey = "captcha_config"

// ReadCaptcha reads the entire versioned state in one database snapshot.
func (r *SettingsRepository) ReadCaptcha(ctx context.Context) (json.RawMessage, error) {
	value, found, err := r.findOptional(ctx, captchaSettingKey)
	if err != nil || !found {
		return nil, err
	}
	return value.Value, nil
}

// CompareAndSwapCaptcha atomically replaces one settings row, including encrypted
// secrets, active configuration, draft and its test attestations. Competing writers
// never overwrite one another. Nil previous means insert only if the row is absent.
func (r *SettingsRepository) CompareAndSwapCaptcha(ctx context.Context, previous, next json.RawMessage) (bool, error) {
	if !json.Valid(next) || len(next) > 64<<10 {
		return false, model.ErrInvalidInput
	}
	// Do not let even an operator-enabled GORM logger print encrypted configuration.
	db := r.db.WithContext(ctx).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	var result *gorm.DB
	if db.Name() == "postgres" {
		if previous == nil {
			result = db.Exec("INSERT INTO settings (key,value) VALUES (?,?::jsonb) ON CONFLICT (key) DO NOTHING", captchaSettingKey, string(next))
		} else {
			result = db.Exec("UPDATE settings SET value=?::jsonb, updated_at=CURRENT_TIMESTAMP WHERE key=? AND value=?::jsonb", string(next), captchaSettingKey, string(previous))
		}
	} else {
		if previous == nil {
			result = db.Exec("INSERT INTO settings (key,value) VALUES (?,?) ON CONFLICT (key) DO NOTHING", captchaSettingKey, string(next))
		} else {
			result = db.Exec("UPDATE settings SET value=?, updated_at=CURRENT_TIMESTAMP WHERE key=? AND value=?", string(next), captchaSettingKey, string(previous))
		}
	}
	if result.Error != nil {
		return false, fmt.Errorf("persist captcha snapshot: %w", model.ErrStorage)
	}
	return result.RowsAffected == 1, nil
}
