package model

import "time"

// LoginEvent is one successful sign-in that created a web session.
type LoginEvent struct {
	ID        uint64 `gorm:"primaryKey"`
	UserID    uint64
	IP        string
	UserAgent string
	CreatedAt time.Time
}

// TokenUse is a batch of authenticated requests made with one token, written
// as a single update.
type TokenUse struct {
	TokenID  uint64
	UserID   uint64
	Kind     string
	At       time.Time
	IP       string
	Requests int64
}

// APIUsageDay totals one account's API-token requests on one UTC day.
type APIUsageDay struct {
	UserID   uint64
	Day      string
	Requests int64
}
