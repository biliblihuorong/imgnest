package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// Settings bounds mirror the repository reader for trash retention.
const (
	maxTrashDays   = 36500
	maxSiteNameRun = 100
)

// GetSettings returns the full site configuration with documented defaults
// for keys that were never stored.
func (s *AdminService) GetSettings(ctx context.Context) (AdminSettingsView, error) {
	if err := ctx.Err(); err != nil {
		return AdminSettingsView{}, fmt.Errorf("read admin settings: %w", err)
	}
	values, err := s.deps.Settings.ReadSettings(ctx)
	if err != nil {
		return AdminSettingsView{}, fmt.Errorf("read admin settings: %w", err)
	}
	view := AdminSettingsView{SiteName: DefaultSiteName, TrashDays: 7, APIEnabled: true, AvatarProvider: model.DefaultAvatarProvider}
	if value, ok := values["site_name"]; ok {
		if err := decodeSetting(value, &view.SiteName); err != nil {
			return AdminSettingsView{}, fmt.Errorf("decode site name: %w", ErrInvalidInput)
		}
	}
	for key, target := range map[string]*bool{
		"registration_enabled":       &view.RegistrationEnabled,
		"guest_upload_enabled":       &view.GuestUploadEnabled,
		"gallery_enabled":            &view.GalleryEnabled,
		"api_enabled":                &view.APIEnabled,
		"gallery_public_albums_only": &view.GalleryPublicAlbumsOnly,
	} {
		if value, ok := values[key]; ok {
			if err := decodeSetting(value, target); err != nil {
				return AdminSettingsView{}, fmt.Errorf("decode %s: %w", key, ErrInvalidInput)
			}
		}
	}
	if value, ok := values["trash_days"]; ok {
		var days int64
		if err := decodeSetting(value, &days); err != nil || days < 0 || days > maxTrashDays {
			return AdminSettingsView{}, fmt.Errorf("decode trash retention: %w", ErrInvalidInput)
		}
		view.TrashDays = int(days)
	}
	for key, target := range map[string]*uint64{
		"guest_group_id":   &view.GuestGroupID,
		"default_group_id": &view.DefaultGroupID,
	} {
		if value, ok := values[key]; ok {
			if err := decodeSetting(value, target); err != nil {
				return AdminSettingsView{}, fmt.Errorf("decode %s: %w", key, ErrInvalidInput)
			}
		}
	}
	if value, ok := values["avatar_provider"]; ok {
		var provider string
		if err := decodeSetting(value, &provider); err != nil {
			return AdminSettingsView{}, fmt.Errorf("decode avatar provider: %w", ErrInvalidInput)
		}
		// A corrupt or unsupported stored value degrades to the default
		// provider instead of failing the whole settings read; unknown hosts
		// are never contacted.
		if model.ValidAvatarProvider(provider) {
			view.AvatarProvider = provider
		}
	}
	return view, nil
}

// PutSettings validates and writes only the supplied keys, then returns the
// full configuration.
func (s *AdminService) PutSettings(ctx context.Context, patch SettingsPatch) (AdminSettingsView, error) {
	if err := ctx.Err(); err != nil {
		return AdminSettingsView{}, fmt.Errorf("write admin settings: %w", err)
	}
	writes := map[string]json.RawMessage{}
	if patch.SiteName != nil {
		name := strings.TrimSpace(*patch.SiteName)
		if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > maxSiteNameRun {
			return AdminSettingsView{}, ErrInvalidInput
		}
		encoded, err := json.Marshal(name)
		if err != nil {
			return AdminSettingsView{}, ErrInvalidInput
		}
		writes["site_name"] = encoded
	}
	for key, value := range map[string]*bool{
		"registration_enabled":       patch.RegistrationEnabled,
		"guest_upload_enabled":       patch.GuestUploadEnabled,
		"gallery_enabled":            patch.GalleryEnabled,
		"api_enabled":                patch.APIEnabled,
		"gallery_public_albums_only": patch.GalleryPublicAlbumsOnly,
	} {
		if value == nil {
			continue
		}
		encoded, err := json.Marshal(*value)
		if err != nil {
			return AdminSettingsView{}, ErrInvalidInput
		}
		writes[key] = encoded
	}
	if patch.TrashDays != nil {
		if *patch.TrashDays < 0 || *patch.TrashDays > maxTrashDays {
			return AdminSettingsView{}, ErrInvalidInput
		}
		encoded, err := json.Marshal(*patch.TrashDays)
		if err != nil {
			return AdminSettingsView{}, ErrInvalidInput
		}
		writes["trash_days"] = encoded
	}
	if patch.AvatarProvider != nil {
		if !model.ValidAvatarProvider(*patch.AvatarProvider) {
			return AdminSettingsView{}, ErrInvalidInput
		}
		encoded, err := json.Marshal(*patch.AvatarProvider)
		if err != nil {
			return AdminSettingsView{}, ErrInvalidInput
		}
		writes["avatar_provider"] = encoded
	}
	// The guest setting must name a guest group and the default setting an
	// ordinary one; otherwise signups would land in the guest group or guest
	// uploads would draw on an account group's rules and quota.
	if patch.GuestGroupID != nil && *patch.GuestGroupID != 0 {
		group, err := s.deps.Groups.FindGroup(ctx, *patch.GuestGroupID)
		if err != nil {
			return AdminSettingsView{}, fmt.Errorf("find guest group: %w", err)
		}
		if !group.IsGuest {
			return AdminSettingsView{}, ErrInvalidInput
		}
	}
	if patch.DefaultGroupID != nil {
		if *patch.DefaultGroupID == 0 {
			return AdminSettingsView{}, ErrInvalidInput
		}
		group, err := s.deps.Groups.FindGroup(ctx, *patch.DefaultGroupID)
		if err != nil {
			return AdminSettingsView{}, fmt.Errorf("find default group: %w", err)
		}
		if group.IsGuest {
			return AdminSettingsView{}, ErrInvalidInput
		}
	}
	if patch.GuestGroupID != nil {
		encoded, err := json.Marshal(*patch.GuestGroupID)
		if err != nil {
			return AdminSettingsView{}, ErrInvalidInput
		}
		writes["guest_group_id"] = encoded
	}
	if patch.DefaultGroupID != nil {
		encoded, err := json.Marshal(*patch.DefaultGroupID)
		if err != nil {
			return AdminSettingsView{}, ErrInvalidInput
		}
		writes["default_group_id"] = encoded
	}
	if len(writes) > 0 {
		if err := s.deps.Settings.UpdateSettings(ctx, writes); err != nil {
			return AdminSettingsView{}, fmt.Errorf("write admin settings: %w", err)
		}
	}
	return s.GetSettings(ctx)
}

func decodeSetting(value json.RawMessage, target any) error {
	return json.Unmarshal(value, target)
}
