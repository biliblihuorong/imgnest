package extension

import (
	"context"
	"encoding/json"
)

// MaxUserAssetBytes bounds a single private account-extension asset.
const MaxUserAssetBytes = 4 << 20

// SettingsUser is server-derived account and group metadata for a plugin.
type SettingsUser struct {
	ID        uint64
	GroupID   uint64
	GroupName string
}

// UserSettingsData is one account's saved plugin settings. Version changes
// whenever its settings or asset changes; missing settings have version zero.
type UserSettingsData struct {
	User    SettingsUser
	Values  json.RawMessage
	Version uint64
}

// UserSettingsStore is read-only and scoped to the receiving plugin. Assets
// are private, bounded binary data; an unset asset returns empty data and MIME.
type UserSettingsStore interface {
	Read(ctx context.Context, userID uint64) (UserSettingsData, error)
	Asset(ctx context.Context, userID uint64) ([]byte, string, error)
}

// UserSettingsUser receives durable account storage before serving starts.
type UserSettingsUser interface {
	UseUserSettings(store UserSettingsStore)
}

// UserSettingsSchema describes a card in the account settings page. ReadOnly
// denies every user mutation, including asset replacement and removal.
// Fields are flat and cannot contain secrets or administrator option sources.
type UserSettingsSchema struct {
	SettingsSchema
	ReadOnly   bool
	Notice     string
	AssetLabel string
}

// UserConfigurable contributes a personal settings card. The core validates
// and stores values; the plugin validates business rules without mutating state.
// Hidden cards and their assets cannot be accessed through the account API.
type UserConfigurable interface {
	UserSettingsSchema(ctx context.Context, user SettingsUser) (UserSettingsSchema, bool)
	ValidateUserSettings(ctx context.Context, user SettingsUser, values json.RawMessage) error
}

// UserAssetProcessor validates and normalizes an uploaded private asset.
// It must inspect real file content, strip metadata and enforce image limits.
// Returning an error leaves the previously stored asset intact.
type UserAssetProcessor interface {
	PrepareUserAsset(ctx context.Context, user SettingsUser, data []byte) ([]byte, string, error)
}
