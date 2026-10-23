package model

import "encoding/json"

// PluginSettingsUser identifies the enabled account owning personal data.
type PluginSettingsUser struct {
	ID        uint64
	GroupID   uint64
	GroupName string
}

// PluginUserData holds settings and the current revision, without asset bytes.
type PluginUserData struct {
	User    PluginSettingsUser
	Values  json.RawMessage
	Version uint64
}

// MaxPluginUserAssetBytes is the storage limit for a private plugin asset.
const MaxPluginUserAssetBytes = 4 << 20
