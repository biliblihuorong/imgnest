package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type crawlerSettingsStub struct {
	values map[string]json.RawMessage
	err    error
}

func (s *crawlerSettingsStub) ReadSettings(context.Context) (map[string]json.RawMessage, error) {
	return s.values, s.err
}

type galleryStub struct {
	total int
	calls int
	err   error
}

func (g *galleryStub) Gallery(_ context.Context, page, size int) (GalleryPage, error) {
	g.calls++
	if g.err != nil {
		return GalleryPage{}, g.err
	}
	items := []GalleryItem{}
	for i := (page - 1) * size; i < page*size && i < g.total; i++ {
		items = append(items, GalleryItem{Links: ImageLinks{URL: fmt.Sprintf("/i/1/%d.webp", i)}})
	}
	return GalleryPage{Items: items, Total: int64(g.total), Page: page, Size: size}, nil
}

func crawlerFixture(t *testing.T, values map[string]string, gallery *galleryStub) (*CrawlerService, *crawlerSettingsStub, *time.Time) {
	t.Helper()
	raw := map[string]json.RawMessage{}
	for key, value := range values {
		raw[key] = json.RawMessage(value)
	}
	settings := &crawlerSettingsStub{values: raw}
	now := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	svc, err := NewCrawlerService(t.Context(), settings, gallery, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return svc, settings, &now
}

func TestCrawlerRobotsModes(t *testing.T) {
	const origin = "https://img.example"
	for _, tc := range []struct {
		name     string
		values   map[string]string
		contains []string
		absent   []string
	}{
		{name: "default blocks everything", values: nil, contains: []string{"User-agent: *\nDisallow: /\n"}, absent: []string{"Allow", "Sitemap"}},
		{name: "unknown mode blocks everything", values: map[string]string{"robots_mode": `"open"`}, contains: []string{"Disallow: /\n"}, absent: []string{"Allow"}},
		{name: "corrupt mode blocks everything", values: map[string]string{"robots_mode": `7`}, contains: []string{"Disallow: /\n"}, absent: []string{"Allow"}},
		{
			name:     "public without sitemap",
			values:   map[string]string{"robots_mode": `"public"`, "gallery_enabled": `true`},
			contains: []string{"Allow: /$\n", "Allow: /home\n", "Allow: /gallery\n", "Allow: /assets/\n", "Allow: /i/\n", "Allow: /t/\n", "Disallow: /\n"},
			absent:   []string{"Sitemap", "/api", "/random"},
		},
		{
			name:     "public with sitemap",
			values:   map[string]string{"robots_mode": `"public"`, "gallery_enabled": `true`, "sitemap_enabled": `true`},
			contains: []string{"Sitemap: https://img.example/sitemap.xml\n"},
		},
		{
			name:   "sitemap needs the gallery",
			values: map[string]string{"robots_mode": `"public"`, "sitemap_enabled": `true`},
			absent: []string{"Sitemap"},
		},
		{
			name:     "custom verbatim with normalized line ends",
			values:   map[string]string{"robots_mode": `"custom"`, "robots_custom": `"User-agent: Googlebot\r\nAllow: /"`},
			contains: []string{"User-agent: Googlebot\nAllow: /\n"},
			absent:   []string{"\r", "Disallow"},
		},
		{name: "empty custom blocks everything", values: map[string]string{"robots_mode": `"custom"`, "robots_custom": `"  "`}, contains: []string{"Disallow: /\n"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _ := crawlerFixture(t, tc.values, &galleryStub{})
			text := svc.Robots(t.Context(), origin)
			for _, want := range tc.contains {
				if !strings.Contains(text, want) {
					t.Fatalf("robots=%q lacks %q", text, want)
				}
			}
			for _, unwanted := range tc.absent {
				if strings.Contains(text, unwanted) {
					t.Fatalf("robots=%q contains %q", text, unwanted)
				}
			}
		})
	}
}

func TestCrawlerRobotsFailsClosed(t *testing.T) {
	svc, settings, _ := crawlerFixture(t, map[string]string{"robots_mode": `"public"`}, &galleryStub{})
	settings.err = errors.New("database down")
	if text := svc.Robots(t.Context(), "http://x"); text != robotsBlockAllText {
		t.Fatalf("robots on read failure=%q", text)
	}
}

func TestCrawlerSitemapServedOnlyWhenEnabled(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values map[string]string
	}{
		{name: "switch off", values: map[string]string{"robots_mode": `"public"`, "gallery_enabled": `true`}},
		{name: "gallery off", values: map[string]string{"robots_mode": `"public"`, "sitemap_enabled": `true`}},
		{name: "robots blocks the site", values: map[string]string{"gallery_enabled": `true`, "sitemap_enabled": `true`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gallery := &galleryStub{total: 3}
			svc, _, _ := crawlerFixture(t, tc.values, gallery)
			if _, err := svc.Sitemap(t.Context()); !errors.Is(err, ErrNotFound) {
				t.Fatalf("err=%v want not found", err)
			}
			if gallery.calls != 0 {
				t.Fatalf("gallery read %d times while the sitemap is off", gallery.calls)
			}
		})
	}
}

func TestCrawlerSitemapCapsAndCaches(t *testing.T) {
	enabled := map[string]string{"robots_mode": `"custom"`, "gallery_enabled": `true`, "sitemap_enabled": `true`}
	gallery := &galleryStub{total: 1500}
	svc, _, now := crawlerFixture(t, enabled, gallery)
	sitemap, err := svc.Sitemap(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(sitemap.ImageURLs) != sitemapMaxImages || sitemap.ImageURLs[0] != "/i/1/0.webp" {
		t.Fatalf("images=%d first=%q", len(sitemap.ImageURLs), sitemap.ImageURLs[0])
	}
	if gallery.calls != sitemapMaxImages/galleryMaxSize {
		t.Fatalf("gallery pages read=%d", gallery.calls)
	}
	if _, err := svc.Sitemap(t.Context()); err != nil || gallery.calls != sitemapMaxImages/galleryMaxSize {
		t.Fatalf("cached read err=%v calls=%d", err, gallery.calls)
	}
	*now = now.Add(sitemapTTL)
	gallery.total = 2
	sitemap, err = svc.Sitemap(t.Context())
	if err != nil || len(sitemap.ImageURLs) != 2 {
		t.Fatalf("refreshed err=%v images=%d", err, len(sitemap.ImageURLs))
	}
}

func TestCrawlerSitemapPropagatesGalleryFailure(t *testing.T) {
	enabled := map[string]string{"robots_mode": `"public"`, "gallery_enabled": `true`, "sitemap_enabled": `true`}
	svc, _, _ := crawlerFixture(t, enabled, &galleryStub{err: errors.New("db")})
	if _, err := svc.Sitemap(t.Context()); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestAdminSettingsCrawlerFields(t *testing.T) {
	svc, _, _, _, _, _, settings := adminServiceFixture(t, AdminDependencies{})
	mode, custom, sitemap := RobotsCustom, "User-agent: *\nDisallow: /api/\n", true
	if _, err := svc.PutSettings(t.Context(), SettingsPatch{RobotsMode: &mode, RobotsCustom: &custom, SitemapEnabled: &sitemap}); err != nil {
		t.Fatal(err)
	}
	written := settings.written[len(settings.written)-1]
	if string(written["robots_mode"]) != `"custom"` || string(written["robots_custom"]) != `"User-agent: *\nDisallow: /api/\n"` || string(written["sitemap_enabled"]) != `true` {
		t.Fatalf("written=%s", written)
	}
	settings.values = written
	view, err := svc.GetSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if view.RobotsMode != RobotsCustom || view.RobotsCustom != custom || !view.SitemapEnabled {
		t.Fatalf("view=%+v", view)
	}
	for _, patch := range []SettingsPatch{
		{RobotsMode: ptr("open")},
		{RobotsMode: ptr("")},
		{RobotsCustom: ptr(strings.Repeat("a", MaxRobotsCustomBytes+1))},
		{RobotsCustom: ptr("User-agent: *\x00")},
		{RobotsCustom: ptr("\xff")},
	} {
		if _, err := svc.PutSettings(t.Context(), patch); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("patch %+v err=%v", patch, err)
		}
	}
	if _, err := svc.PutSettings(t.Context(), SettingsPatch{RobotsCustom: ptr(strings.Repeat("a", MaxRobotsCustomBytes))}); err != nil {
		t.Fatalf("custom at the limit: %v", err)
	}
}
