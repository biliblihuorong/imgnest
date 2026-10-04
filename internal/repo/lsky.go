package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// LskyRepository reads the settings keys and counters consumed by the Lsky v1
// compatibility layer. It never exposes setting values themselves.
type LskyRepository struct{ db *gorm.DB }

// NewLskyRepository creates the v1 support persistence adapter.
func NewLskyRepository(ctx context.Context, db *gorm.DB) (*LskyRepository, error) {
	if err := checkDatabase(ctx, db); err != nil {
		return nil, err
	}
	return &LskyRepository{db: db}, nil
}

// APIEnabled reports whether the administrator allows the v1 API. A missing
// key keeps the migration default of enabled.
func (r *LskyRepository) APIEnabled(ctx context.Context) (bool, error) {
	value, found, err := settingValue(ctx, r.db, "api_enabled")
	if err != nil || !found {
		return true, err
	}
	enabled, err := decodeBool(value, true)
	if err != nil {
		return false, fmt.Errorf("decode api switch: %w", err)
	}
	return enabled, nil
}

// GuestUploadEnabled reports whether anonymous uploads are allowed. A missing
// key keeps the migration default of disabled.
func (r *LskyRepository) GuestUploadEnabled(ctx context.Context) (bool, error) {
	value, found, err := settingValue(ctx, r.db, "guest_upload_enabled")
	if err != nil || !found {
		return false, err
	}
	enabled, err := decodeBool(value, false)
	if err != nil {
		return false, fmt.Errorf("decode guest upload switch: %w", err)
	}
	return enabled, nil
}

// GuestGroup resolves the group governing anonymous uploads: the configured
// guest_group_id wins when it exists, otherwise the is_guest group. The second
// return value is false when the site has no usable guest group.
func (r *LskyRepository) GuestGroup(ctx context.Context) (model.Group, bool, error) {
	return resolveGuestGroup(ctx, r.db)
}

// RegisteredIP returns the recorded registration address of one account.
func (r *LskyRepository) RegisteredIP(ctx context.Context, userID uint64) (string, error) {
	if err := checkRecordID(ctx, userID); err != nil {
		return "", fmt.Errorf("read registered IP: %w", err)
	}
	var ip string
	err := r.db.WithContext(ctx).Table("users").
		Select("registered_ip").
		Where("id = ?", userID).
		Scan(&ip).Error
	if err != nil {
		return "", databaseError("read registered IP", err)
	}
	return ip, nil
}

// GroupCapacityBytes returns one group's byte capacity; zero means unlimited.
func (r *LskyRepository) GroupCapacityBytes(ctx context.Context, groupID uint64) (int64, error) {
	if err := checkRecordID(ctx, groupID); err != nil {
		return 0, fmt.Errorf("read group capacity: %w", err)
	}
	var group model.Group
	if err := r.db.WithContext(ctx).First(&group, "id = ?", groupID).Error; err != nil {
		return 0, repositoryError("read group capacity", err)
	}
	return group.CapacityBytes, nil
}

// CountActiveImages counts one owner's active (non-trashed) images.
func (r *LskyRepository) CountActiveImages(ctx context.Context, userID uint64) (int64, error) {
	if err := checkRecordID(ctx, userID); err != nil {
		return 0, fmt.Errorf("count owner images: %w", err)
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.Image{}).
		Where("user_id = ? AND state = ?", userID, model.ImageStateActive).
		Count(&count).Error; err != nil {
		return 0, repositoryError("count owner images", err)
	}
	return count, nil
}

// resolveGuestGroup resolves the guest group inside the current transaction so
// reservations and policy grants observe one consistent configuration.
func resolveGuestGroup(ctx context.Context, tx *gorm.DB) (model.Group, bool, error) {
	value, found, err := settingValue(ctx, tx, "guest_group_id")
	if err != nil {
		return model.Group{}, false, err
	}
	if found {
		var explicit uint64
		if err := json.Unmarshal(value, &explicit); err != nil {
			return model.Group{}, false, fmt.Errorf("decode guest group setting: %w", model.ErrInvalidInput)
		}
		if explicit > 0 {
			var group model.Group
			if err := tx.WithContext(ctx).First(&group, "id = ?", explicit).Error; err == nil {
				return group, true, nil
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return model.Group{}, false, repositoryError("find guest group", err)
			}
		}
	}
	var fallback model.Group
	if err := tx.WithContext(ctx).First(&fallback, "is_guest = ?", true).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.Group{}, false, nil
		}
		return model.Group{}, false, repositoryError("find guest group", err)
	}
	return fallback, true, nil
}

func settingValue(ctx context.Context, db *gorm.DB, key string) (json.RawMessage, bool, error) {
	var setting model.Setting
	err := db.WithContext(ctx).First(&setting, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, databaseError("find setting", err)
	}
	return setting.Value, true, nil
}

func decodeBool(value json.RawMessage, fallback bool) (bool, error) {
	var decoded *bool
	if err := json.Unmarshal(value, &decoded); err != nil {
		return false, model.ErrInvalidInput
	}
	if decoded == nil {
		return fallback, nil
	}
	return *decoded, nil
}
