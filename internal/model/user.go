// Package model contains shared persistence values and domain errors.
package model

import "time"

// User roles and statuses define account authorization and availability.
const (
	UserRoleAdmin      = "admin"
	UserRoleUser       = "user"
	UserStatusEnabled  = "enabled"
	UserStatusDisabled = "disabled"
)

// User stores account credentials and authorization attributes.
type User struct {
	// AuthVersion is internal account revocation state, never an editable or
	// public profile field. Security edits advance it even when later undone.
	AuthVersion uint64 `json:"-"`
	ID          uint64 `gorm:"primaryKey"`
	// DisplayName is the optional self-chosen profile name; an empty value
	// means clients fall back to the username.
	DisplayName  string
	GroupID      uint64
	Username     string
	Email        string
	PasswordHash string
	Role         string
	Status       string
	UsedBytes    int64
	// RegisteredIP records the client address of account creation; only the
	// account owner's profile read may return it.
	RegisteredIP string
	// PublicID is the random base62 handle used in anonymous URLs; nil until
	// the account first needs one.
	PublicID  *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// UserChanges is the explicit allowlist of administrator-editable account
// fields. Credentials, counters, timestamps and registration metadata are not
// part of this value and cannot be mass assigned.
type UserChanges struct {
	Username    *string
	Email       *string
	DisplayName *string
	Role        *string
	Status      *string
	GroupID     *uint64
}

// AccountState pins the identity and authorization values checked at login.
// A credential verified before an administrative change must not create a
// fresh session after that change has revoked the account's existing tokens.
type AccountState struct {
	AuthVersion uint64
	Username    string
	Email       string
	Role        string
	Status      string
	GroupID     uint64
}
