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
// registration switch; the site name reads the administrator-configured
// setting and falls back to DefaultSiteName while none is stored.
func (s *UserService) Site(ctx context.Context) (SiteView, error) {
	if err := ctx.Err(); err != nil {
		return SiteView{}, fmt.Errorf("read site view: %w", err)
	}
	enabled, err := s.settings.RegistrationEnabled(ctx)
	if err != nil {
		return SiteView{}, fmt.Errorf("read registration setting: %w", err)
	}
	name, err := s.settings.SiteName(ctx)
	if err != nil {
		return SiteView{}, fmt.Errorf("read site name setting: %w", err)
	}
	if name == "" {
		name = DefaultSiteName
	}
	return SiteView{SiteName: name, RegisterEnabled: enabled}, nil
}
