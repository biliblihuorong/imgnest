package service

import (
	"context"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// SettingsRepository supplies the site rules used by account operations.
type SettingsRepository interface {
	RegistrationEnabled(context.Context) (bool, error)
	DefaultGroupID(ctx context.Context) (uint64, error)
	// SiteName returns the configured public site name; an empty value means
	// no name was stored and the caller falls back to DefaultSiteName.
	SiteName(ctx context.Context) (string, error)
	// GalleryEnabled returns the public gallery switch; a closed gallery
	// serves empty pages instead of authorization errors.
	GalleryEnabled(ctx context.Context) (bool, error)
	// AvatarConfig returns the site-wide avatar provider selection; Provider
	// is always a supported enum value and Version identifies config updates.
	AvatarConfig(context.Context) (model.AvatarConfig, error)
}
