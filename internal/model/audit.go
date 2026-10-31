package model

import "time"

// AuditLog is one recorded state-changing API request.
type AuditLog struct {
	ID uint64 `gorm:"primaryKey"`
	// UserID is the signed-in account, or zero for an anonymous request
	// such as a failed sign-in. It is kept after the account is deleted.
	UserID    uint64
	UserName  string
	IP        string
	Method    string
	Route     string
	Target    string
	Status    int
	RequestID string
	CreatedAt time.Time
}

// AuditQuery filters a page of audit entries, newest first.
type AuditQuery struct {
	Page, Size int
	// UserID keeps one account's entries; zero keeps all.
	UserID uint64
	// FailedOnly keeps requests answered with a 4xx or 5xx status.
	FailedOnly bool
}
