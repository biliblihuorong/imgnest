package http_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	httpapi "github.com/biliblihuorong/imgnest/internal/http"
	"github.com/biliblihuorong/imgnest/internal/service"
)

type crawlerStub struct {
	origin  string
	sitemap service.Sitemap
	err     error
}

func (s *crawlerStub) Robots(_ context.Context, origin string) string {
	s.origin = origin
	return "User-agent: *\nDisallow: /\n"
}

func (s *crawlerStub) Sitemap(context.Context) (service.Sitemap, error) { return s.sitemap, s.err }

func crawlerRouter(t *testing.T, crawler *crawlerStub, proxies []string) http.Handler {
	t.Helper()
	router, err := httpapi.NewRouter(t.Context(), httpapi.Dependencies{
		Users:   &usersStub{},
		Tokens:  &tokensStub{},
		Logger:  slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Server:  config.Server{TrustedProxies: proxies},
		Now:     time.Now,
		Health:  func(context.Context) error { return nil },
		Web:     spaFixture(),
		Crawler: crawler,
	})
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func crawlerRequest(router http.Handler, path, remote, proto string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.Host = "img.example"
	r.RemoteAddr = remote
	if proto != "" {
		r.Header.Set("X-Forwarded-Proto", proto)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestRobotsServedAsText(t *testing.T) {
	crawler := &crawlerStub{}
	router := crawlerRouter(t, crawler, []string{"10.0.0.0/8"})
	w := crawlerRequest(router, "/robots.txt", "192.0.2.1:1234", "https")
	if w.Code != http.StatusOK || w.Body.String() != "User-agent: *\nDisallow: /\n" {
		t.Fatalf("HTTP=%d body=%q", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("content type %q", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "public, max-age=3600" {
		t.Fatalf("cache control %q", got)
	}
	if crawler.origin != "http://img.example" {
		t.Fatalf("untrusted X-Forwarded-Proto honoured: origin=%q", crawler.origin)
	}
	crawlerRequest(router, "/robots.txt", "10.1.2.3:1234", "https")
	if crawler.origin != "https://img.example" {
		t.Fatalf("trusted proxy origin=%q", crawler.origin)
	}
}

func TestSitemapListsGalleryImages(t *testing.T) {
	crawler := &crawlerStub{sitemap: service.Sitemap{ImageURLs: []string{"/i/1/a.webp", "https://cdn.example/b.png?x=1&y=<2>"}}}
	w := crawlerRequest(crawlerRouter(t, crawler, nil), "/sitemap.xml", "192.0.2.1:1234", "")
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP=%d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/xml; charset=utf-8" {
		t.Fatalf("content type %q", got)
	}
	body := w.Body.String()
	for _, want := range []string{
		`xmlns:image="http://www.google.com/schemas/sitemap-image/1.1"`,
		"<loc>http://img.example/home</loc>",
		"<loc>http://img.example/gallery</loc>",
		"<image:loc>http://img.example/i/1/a.webp</image:loc>",
		"<image:loc>https://cdn.example/b.png?x=1&amp;y=&lt;2&gt;</image:loc>",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("sitemap lacks %q:\n%s", want, body)
		}
	}
}

func TestSitemapHiddenWhenOffOrFailing(t *testing.T) {
	for _, err := range []error{service.ErrNotFound, errors.New("database down")} {
		w := crawlerRequest(crawlerRouter(t, &crawlerStub{err: err}, nil), "/sitemap.xml", "192.0.2.1:1234", "")
		if w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "database") {
			t.Fatalf("err=%v HTTP=%d body=%q", err, w.Code, w.Body.String())
		}
		if got := w.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("missing sitemap cached: %q", got)
		}
	}
}

func TestCrawlerFilesNeverFallBackToSPA(t *testing.T) {
	router := spaRouter(t, spaFixture())
	for _, path := range []string{"/robots.txt", "/sitemap.xml"} {
		w := request(t, router, http.MethodGet, path, "", "")
		if w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "spa-shell") {
			t.Fatalf("GET %s without crawler: HTTP=%d body=%q", path, w.Code, w.Body.String())
		}
	}
}

func TestNoindexOutsidePublicPages(t *testing.T) {
	router := spaRouter(t, spaFixture())
	for path, noindex := range map[string]bool{
		"/":             false,
		"/home":         false,
		"/gallery":      false,
		"/gallery/":     false,
		"/login":        true,
		"/admin/users":  true,
		"/images":       true,
		"/api/site":     true,
		"/random/a/b":   true,
		"/healthz":      false,
		"/favicon.svg":  false,
		"/galleryextra": true,
	} {
		w := request(t, router, http.MethodGet, path, "", "")
		got := w.Header().Get("X-Robots-Tag")
		if noindex && got != "noindex, nofollow" || !noindex && got != "" {
			t.Fatalf("GET %s: X-Robots-Tag=%q want noindex=%v", path, got, noindex)
		}
	}
}
