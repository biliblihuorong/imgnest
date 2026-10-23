package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var userPluginName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// PluginUserSettingsRepository stores one private settings/asset record per
// plugin and owner. Independent updates never replace the other field.
type PluginUserSettingsRepository struct {
	db  *gorm.DB
	now func() time.Time
}

// NewPluginUserSettingsRepository constructs the account-extension store.
func NewPluginUserSettingsRepository(ctx context.Context, db *gorm.DB, now func() time.Time) (*PluginUserSettingsRepository, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if db == nil || now == nil {
		return nil, errors.New("create user plugin store: missing dependencies")
	}
	return &PluginUserSettingsRepository{db: db, now: now}, nil
}

func checkUserPluginKey(plugin string, userID uint64) error {
	if !userPluginName.MatchString(plugin) || userID == 0 {
		return model.ErrInvalidInput
	}
	return nil
}

// Read returns current account metadata and settings without reading its Logo.
func (r *PluginUserSettingsRepository) Read(ctx context.Context, plugin string, userID uint64) (model.PluginUserData, error) {
	if err := checkUserPluginKey(plugin, userID); err != nil {
		return model.PluginUserData{}, err
	}
	var row struct {
		ID, GroupID, Version     uint64
		GroupName, Status, Value string
	}
	err := r.db.WithContext(ctx).Raw(`SELECT u.id, u.group_id, g.name AS group_name, u.status,
		COALESCE(p.value, '{}') AS value, COALESCE(p.version, 0) AS version
		FROM users u JOIN groups g ON g.id=u.group_id
		LEFT JOIN plugin_user_settings p ON p.user_id=u.id AND p.plugin=? WHERE u.id=?`, plugin, userID).Scan(&row).Error
	if err != nil {
		return model.PluginUserData{}, repositoryError("read user plugin settings", err)
	}
	if row.ID == 0 {
		return model.PluginUserData{}, model.ErrNotFound
	}
	if row.Status != model.UserStatusEnabled {
		return model.PluginUserData{}, model.ErrForbidden
	}
	return model.PluginUserData{User: model.PluginSettingsUser{ID: row.ID, GroupID: row.GroupID, GroupName: row.GroupName}, Values: json.RawMessage(row.Value), Version: row.Version}, nil
}

// Asset returns only this plugin/owner's private asset; an unset one is empty.
func (r *PluginUserSettingsRepository) Asset(ctx context.Context, plugin string, userID uint64) ([]byte, string, error) {
	if _, err := r.Read(ctx, plugin, userID); err != nil {
		return nil, "", err
	}
	var row struct {
		Asset     []byte
		AssetMIME string
	}
	if err := r.db.WithContext(ctx).Table("plugin_user_settings").Select("asset, asset_mime").Where("plugin = ? AND user_id = ?", plugin, userID).Scan(&row).Error; err != nil {
		return nil, "", repositoryError("read user plugin asset", err)
	}
	return row.Asset, row.AssetMIME, nil
}

// SaveValues atomically replaces settings while preserving the private asset.
func (r *PluginUserSettingsRepository) SaveValues(ctx context.Context, plugin string, userID, groupID uint64, values json.RawMessage) error {
	var object map[string]json.RawMessage
	if len(values) > 65536 || json.Unmarshal(values, &object) != nil || object == nil {
		return model.ErrInvalidInput
	}
	return r.write(ctx, plugin, userID, groupID, func(tx *gorm.DB) error {
		return tx.Exec(`INSERT INTO plugin_user_settings (plugin,user_id,value,version,updated_at) VALUES (?,?,?,1,?)
			ON CONFLICT (plugin,user_id) DO UPDATE SET value=excluded.value, version=plugin_user_settings.version+1, updated_at=excluded.updated_at`, plugin, userID, string(values), r.now().UTC()).Error
	})
}

// SaveAsset atomically replaces/removes an asset while preserving settings.
func (r *PluginUserSettingsRepository) SaveAsset(ctx context.Context, plugin string, userID, groupID uint64, data []byte, mime string) error {
	if len(data) > model.MaxPluginUserAssetBytes || (len(data) == 0 && mime != "") || (len(data) > 0 && mime != "image/png" && mime != "image/jpeg" && mime != "image/webp") {
		return model.ErrInvalidInput
	}
	if len(data) == 0 {
		data = nil
	}
	return r.write(ctx, plugin, userID, groupID, func(tx *gorm.DB) error {
		return tx.Exec(`INSERT INTO plugin_user_settings (plugin,user_id,asset,asset_mime,version,updated_at) VALUES (?,?,?,?,1,?)
			ON CONFLICT (plugin,user_id) DO UPDATE SET asset=excluded.asset,asset_mime=excluded.asset_mime,version=plugin_user_settings.version+1,updated_at=excluded.updated_at`, plugin, userID, data, mime, r.now().UTC()).Error
	})
}

func (r *PluginUserSettingsRepository) write(ctx context.Context, plugin string, userID, groupID uint64, write func(*gorm.DB) error) error {
	if err := checkUserPluginKey(plugin, userID); err != nil {
		return err
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id, group_id, status").First(&user, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.ErrNotFound
			}
			return err
		}
		if user.GroupID != groupID || user.Status != model.UserStatusEnabled {
			return model.ErrForbidden
		}
		return write(tx)
	})
	if err != nil {
		return fmt.Errorf("write user plugin data: %w", repositoryError("write user plugin data", err))
	}
	return nil
}
