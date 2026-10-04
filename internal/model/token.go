package model

import "time"

// Token kinds distinguish web sessions from user-managed API credentials.
const (
	TokenKindWeb = "web"
	TokenKindAPI = "api"
)

// Token stores a bearer-secret digest; plaintext tokens never enter this model.
type Token struct {
	ID         uint64 `gorm:"primaryKey"`
	UserID     uint64
	Name       string
	TokenHash  string
	Kind       string
	Abilities  []string `gorm:"serializer:json"`
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
