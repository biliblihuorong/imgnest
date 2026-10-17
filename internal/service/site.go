package service

import (
	"context"
	"fmt"
)

// DefaultSiteName is the public site title while no site-name setting exists.
const DefaultSiteName = "ImgNest"

// SiteView is the public site descriptor. It deliberately exposes nothing
// beyond the site name, the registration and gallery switches, and which
// email flows work; all other settings keys stay private to administrators.
type SiteView struct {
	SiteName        string `json:"site_name"`
	RegisterEnabled bool   `json:"register_enabled"`
	GalleryEnabled  bool   `json:"gallery_enabled"`
	// EmailVerification means registration needs a code mailed to the address.
	EmailVerification bool `json:"email_verification"`
	// PasswordReset means a forgotten password can be reset by email.
	PasswordReset bool `json:"password_reset"`
	// GuestUpload means visitors can upload from the public page without
	// an account.
	GuestUpload bool `json:"guest_upload"`
}

// GuestUploadSwitch is the optional settings capability behind
// SiteView.GuestUpload; settings without it report guest uploads closed.
type GuestUploadSwitch interface {
	GuestUploadOpen(ctx context.Context) (bool, error)
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
	verification, reset, err := s.emailFeatures(ctx)
	if err != nil {
		return SiteView{}, fmt.Errorf("read email features: %w", err)
	}
	guest := false
	if switcher, ok := s.settings.(GuestUploadSwitch); ok {
		if guest, err = switcher.GuestUploadOpen(ctx); err != nil {
			return SiteView{}, fmt.Errorf("read guest upload setting: %w", err)
		}
	}
	return SiteView{SiteName: name, RegisterEnabled: enabled, GalleryEnabled: gallery,
		EmailVerification: verification && enabled, PasswordReset: reset, GuestUpload: guest}, nil
}
