package model

import "time"

// Token kinds distinguish web sessions from user-managed API credentials.
const (
	TokenKindWeb = "web"
	TokenKindAPI = "api"
)

// TokenGrant carries the verified credential state that must still hold when a token is inserted.
type TokenGrant struct {
	UserID               uint64
	ExpectedPasswordHash string
	ExpectedAccountState *AccountState
	SourceTokenID        uint64
	At                   time.Time
	// Clock, when set, is read again once the repository holds its locks, so
	// a source token that expires while the request waits or processes an
	// image is rejected at commit time instead of at the request's start.
	Clock func() time.Time
}

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
