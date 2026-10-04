package service

import "context"

// SettingsRepository supplies the site rules used by account operations.
type SettingsRepository interface {
	RegistrationEnabled(ctx context.Context) (bool, error)
	DefaultGroupID(ctx context.Context) (uint64, error)
}
