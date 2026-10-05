package model

import (
	"encoding/json"
	"time"
)

// Setting stores one JSON-valued site configuration entry.
type Setting struct {
	Key       string          `gorm:"primaryKey"`
	Value     json.RawMessage `gorm:"serializer:json"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Avatar providers are the only external avatar sources the site may contact.
const (
	AvatarProviderWeavatar = "weavatar"
	AvatarProviderGravatar = "gravatar"
)

// DefaultAvatarProvider is used when no provider was configured or a stored
// value is unsupported; unknown or malformed configuration never selects a
// different external host.
const DefaultAvatarProvider = AvatarProviderWeavatar

// AvatarConfig is the site-wide avatar provider selection plus the version
// clients compare to notice configuration changes.
type AvatarConfig struct {
	Provider string
	Version  int64
}

// ValidAvatarProvider reports whether provider is one of the supported values.
func ValidAvatarProvider(provider string) bool {
	return provider == AvatarProviderWeavatar || provider == AvatarProviderGravatar
}
