package service

import (
	"context"
	"fmt"
)

// DefaultSiteName is the public site title while no site-name setting exists.
const DefaultSiteName = "ImgNest"

// SiteView is the public site descriptor. It deliberately exposes nothing
// beyond the site name and the registration switch; all other settings keys
// stay private to administrators.
type SiteView struct {
	SiteName        string `json:"site_name"`
	RegisterEnabled bool   `json:"register_enabled"`
}

// Site returns the public site view. Registration mirrors the account
// registration switch; the site name falls back to DefaultSiteName because M3
// has no site-name setting yet. When an administrator-configurable name is
// introduced, this method reads it through SettingsRepository.
func (s *UserService) Site(ctx context.Context) (SiteView, error) {
	if err := ctx.Err(); err != nil {
		return SiteView{}, fmt.Errorf("read site view: %w", err)
	}
	enabled, err := s.settings.RegistrationEnabled(ctx)
	if err != nil {
		return SiteView{}, fmt.Errorf("read registration setting: %w", err)
	}
	return SiteView{SiteName: DefaultSiteName, RegisterEnabled: enabled}, nil
}
