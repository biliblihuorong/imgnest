package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
)

type siteSettingStub struct {
	SettingsRepository
	enabled bool
	name    string
	gallery bool
	err     error
}

func (s siteSettingStub) RegistrationEnabled(context.Context) (bool, error) {
	return s.enabled, s.err
}

func (s siteSettingStub) SiteName(context.Context) (string, error) {
	return s.name, s.err
}

func (s siteSettingStub) GalleryEnabled(context.Context) (bool, error) {
	return s.gallery, s.err
}

type siteUserStub struct{ UserRepository }

func TestSiteViewExposesOnlyPublicFields(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled bool
		gallery bool
		setting string
		want    string
	}{
		{"registration closed", false, false, "", DefaultSiteName},
		{"registration open", true, false, "", DefaultSiteName},
		{"configured name", true, true, "My Nest", "My Nest"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			users, err := NewUserService(t.Context(), siteUserStub{}, siteSettingStub{enabled: tc.enabled, gallery: tc.gallery, name: tc.setting})
			if err != nil {
				t.Fatal(err)
			}
			view, err := users.Site(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if view.SiteName != tc.want || view.RegisterEnabled != tc.enabled || view.GalleryEnabled != tc.gallery {
				t.Fatalf("site view %+v", view)
			}
			raw, err := json.Marshal(view)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			if len(fields) != 3 {
				t.Fatalf("site view exposes %d fields: %s", len(fields), raw)
			}
			for _, key := range []string{"site_name", "register_enabled", "gallery_enabled"} {
				if _, ok := fields[key]; !ok {
					t.Fatalf("site view lacks %s: %s", key, raw)
				}
			}
		})
	}
}

func TestSiteViewWrapsSettingFailure(t *testing.T) {
	users, err := NewUserService(t.Context(), siteUserStub{}, siteSettingStub{err: model.ErrStorage})
	if err != nil {
		t.Fatal(err)
	}
	view, err := users.Site(t.Context())
	if !errors.Is(err, model.ErrStorage) {
		t.Fatalf("site view error=%v", err)
	}
	if view != (SiteView{}) {
		t.Fatalf("failed site view returned data %+v", view)
	}
}
