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
	// Login, when set, records a sign-in in the same transaction that
	// creates the web session.
	Login *LoginEvent
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
	// LastUsedIP is the client address of the most recent recorded use.
	LastUsedIP string
	// UseCount counts authenticated requests; uses since the last write are
	// buffered in memory, so a restart can drop up to a minute of them.
	UseCount  int64
	ExpiresAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}
