package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Robots modes stored under the robots_mode settings key.
const (
	// RobotsBlockAll disallows the whole site; it is the default and what
	// every unreadable or unknown setting falls back to.
	RobotsBlockAll = "block_all"
	// RobotsPublic opens only the public home page, the gallery and image
	// paths; the console, API and random links stay disallowed.
	RobotsPublic = "public"
	// RobotsCustom serves the administrator's own robots.txt verbatim.
	RobotsCustom = "custom"
)

const (
	// MaxRobotsCustomBytes bounds the custom robots.txt; crawlers ignore
	// anything past 500 KiB and a hand-written file never needs that much.
	MaxRobotsCustomBytes = 8 << 10
	// sitemapMaxImages is Google's limit of image entries per sitemap URL.
	sitemapMaxImages = 1000
	// sitemapTTL bounds how stale the listed images may be; it also keeps an
	// anonymous client from turning every sitemap fetch into gallery queries.
	sitemapTTL = 10 * time.Minute
)

const robotsBlockAllText = "User-agent: *\nDisallow: /\n"

// CrawlerSettingsReader reads the stored site settings.
type CrawlerSettingsReader interface {
	ReadSettings(context.Context) (map[string]json.RawMessage, error)
}

// GalleryReader pages the public gallery under its own switch and album scope.
type GalleryReader interface {
	Gallery(ctx context.Context, page, size int) (GalleryPage, error)
}

// CrawlerConfig is the search-engine section of the site settings.
type CrawlerConfig struct {
	RobotsMode     string
	RobotsCustom   string
	SitemapEnabled bool
	GalleryEnabled bool
}

// SitemapServed reports whether /sitemap.xml exists: the administrator turned
// it on, the gallery it lists is open, and robots.txt does not block the site.
func (c CrawlerConfig) SitemapServed() bool {
	return c.SitemapEnabled && c.GalleryEnabled && c.RobotsMode != RobotsBlockAll
}

// Sitemap is what /sitemap.xml lists: the public gallery's newest images.
type Sitemap struct {
	// ImageURLs are the gallery images' preferred direct links, newest
	// first. Local-storage links may be site-relative.
	ImageURLs []string
}

// CrawlerService answers robots.txt and the image sitemap.
type CrawlerService struct {
	settings CrawlerSettingsReader
	gallery  GalleryReader
	now      func() time.Time

	mu       sync.Mutex
	images   []string
	cachedAt time.Time
}

// NewCrawlerService constructs the robots.txt and sitemap service.
func NewCrawlerService(ctx context.Context, settings CrawlerSettingsReader, gallery GalleryReader, now func() time.Time) (*CrawlerService, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("construct crawler service: %w", err)
	}
	if settings == nil || gallery == nil {
		return nil, errors.New("construct crawler service: missing dependencies")
	}
	if now == nil {
		now = time.Now
	}
	return &CrawlerService{settings: settings, gallery: gallery, now: now}, nil
}

// Robots returns the robots.txt body for a site served at origin (scheme and
// host, no trailing slash). An unreadable setting blocks the whole site.
func (s *CrawlerService) Robots(ctx context.Context, origin string) string {
	values, err := s.settings.ReadSettings(ctx)
	if err != nil {
		return robotsBlockAllText
	}
	config := crawlerConfig(values)
	switch config.RobotsMode {
	case RobotsPublic:
		var b strings.Builder
		b.WriteString("User-agent: *\n")
		// Google and Bing apply the longest matching rule, so these Allow
		// lines win over the closing Disallow. /assets/ lets crawlers render
		// the single-page app.
		for _, path := range []string{"/$", "/home", "/gallery", "/assets/", "/i/", "/t/"} {
			b.WriteString("Allow: " + path + "\n")
		}
		b.WriteString("Disallow: /\n")
		if config.SitemapServed() {
			b.WriteString("\nSitemap: " + origin + "/sitemap.xml\n")
		}
		return b.String()
	case RobotsCustom:
		if strings.TrimSpace(config.RobotsCustom) == "" {
			return robotsBlockAllText
		}
		text := strings.ReplaceAll(config.RobotsCustom, "\r\n", "\n")
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		return text
	default:
		return robotsBlockAllText
	}
}

// Sitemap returns the public gallery's newest images, or ErrNotFound while
// the sitemap is not served.
func (s *CrawlerService) Sitemap(ctx context.Context) (Sitemap, error) {
	values, err := s.settings.ReadSettings(ctx)
	if err != nil {
		return Sitemap{}, fmt.Errorf("read sitemap settings: %w", err)
	}
	if !crawlerConfig(values).SitemapServed() {
		return Sitemap{}, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.images != nil && s.now().Sub(s.cachedAt) < sitemapTTL {
		return Sitemap{ImageURLs: s.images}, nil
	}
	images := make([]string, 0, galleryMaxSize)
	for page := 1; len(images) < sitemapMaxImages; page++ {
		result, err := s.gallery.Gallery(ctx, page, galleryMaxSize)
		if err != nil {
			return Sitemap{}, fmt.Errorf("list sitemap images: %w", err)
		}
		for _, item := range result.Items {
			if item.Links.URL != "" && len(images) < sitemapMaxImages {
				images = append(images, item.Links.URL)
			}
		}
		if len(result.Items) < galleryMaxSize {
			break
		}
	}
	s.images, s.cachedAt = images, s.now()
	return Sitemap{ImageURLs: images}, nil
}

// crawlerConfig decodes the crawler settings with their defaults; a corrupt
// value degrades to the closed default instead of failing.
func crawlerConfig(values map[string]json.RawMessage) CrawlerConfig {
	config := CrawlerConfig{RobotsMode: RobotsBlockAll}
	var mode string
	if value, ok := values["robots_mode"]; ok && decodeSetting(value, &mode) == nil && validRobotsMode(mode) {
		config.RobotsMode = mode
	}
	var custom string
	if value, ok := values["robots_custom"]; ok && decodeSetting(value, &custom) == nil && validRobotsCustom(custom) {
		config.RobotsCustom = custom
	}
	for key, target := range map[string]*bool{
		"sitemap_enabled": &config.SitemapEnabled,
		"gallery_enabled": &config.GalleryEnabled,
	} {
		var enabled bool
		if value, ok := values[key]; ok && decodeSetting(value, &enabled) == nil {
			*target = enabled
		}
	}
	return config
}

func validRobotsMode(mode string) bool {
	return mode == RobotsBlockAll || mode == RobotsPublic || mode == RobotsCustom
}

// validRobotsCustom accepts bounded UTF-8 text whose only control characters
// are line breaks and tabs.
func validRobotsCustom(text string) bool {
	if len(text) > MaxRobotsCustomBytes || !utf8.ValidString(text) {
		return false
	}
	for _, r := range text {
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' || r == 0x7f {
			return false
		}
	}
	return true
}
