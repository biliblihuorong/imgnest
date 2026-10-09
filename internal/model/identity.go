package model

import "time"

// UserIdentity links an external sign-in subject to a local account.
// Provider is namespaced by the plugin that verified it ("sso:github").
type UserIdentity struct {
	ID        uint64 `gorm:"primaryKey"`
	UserID    uint64
	Provider  string
	Subject   string
	CreatedAt time.Time
}

// TableName pins the migration-owned table name.
func (UserIdentity) TableName() string { return "user_identities" }
