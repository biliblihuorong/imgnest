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
