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
	ID           uint64 `gorm:"primaryKey"`
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
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
