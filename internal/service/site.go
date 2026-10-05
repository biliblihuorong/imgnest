package service

import (
	"context"
	"fmt"
)

// DefaultSiteName is the public site title while no site-name setting exists.
const DefaultSiteName = "ImgNest"

// SiteView is the public site descriptor. It deliberately exposes nothing
// beyond the site name, the registration switch, and the gallery switch; all
// other settings keys stay private to administrators.
type SiteView struct {
	SiteName        string `json:"site_name"`
	RegisterEnabled bool   `json:"register_enabled"`
	GalleryEnabled  bool   `json:"gallery_enabled"`
}

// Site returns the public site view. Registration mirrors the account
// registration switch; the gallery switch drives whether the public gallery
// serves content; the site name reads the administrator-configured setting
// and falls back to DefaultSiteName while none is stored.
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
	gallery, err := s.settings.GalleryEnabled(ctx)
	if err != nil {
		return SiteView{}, fmt.Errorf("read gallery setting: %w", err)
	}
	return SiteView{SiteName: name, RegisterEnabled: enabled, GalleryEnabled: gallery}, nil
}
