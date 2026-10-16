package model

import (
	"encoding/json"
	"time"
)

// Storage identifies an object backend; Config is an encrypted deployment value.
type Storage struct {
	ID        uint64          `gorm:"primaryKey" json:"id"`
	Name      string          `json:"name"`
	Driver    string          `json:"driver"`
	Config    json.RawMessage `gorm:"serializer:json" json:"-"`
	BaseURL   string          `json:"base_url"`
	Enabled   bool            `json:"enabled"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// StorageUpdateGuard binds an admin write to the configuration it validated.
// RequireEmpty is set when the plaintext object location changes, so a
// repository can recheck every image state while holding the upload lock.
type StorageUpdateGuard struct {
	ExpectedDriver string
	ExpectedConfig json.RawMessage
	RequireEmpty   bool
}
