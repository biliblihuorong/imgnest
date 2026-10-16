package model

import "time"

// Email code purposes.
const (
	EmailCodeRegister      = "register"
	EmailCodeResetPassword = "reset_password"
)

// EmailCode is the pending one-time code for one purpose and address. Only
// a hash of the code is stored.
type EmailCode struct {
	ID        uint64
	Purpose   string
	Email     string
	CodeHash  string
	Attempts  int
	ExpiresAt time.Time
	CreatedAt time.Time
}
