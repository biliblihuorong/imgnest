package http_test

import (
	"context"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	httpapi "github.com/biliblihuorong/imgnest/internal/http"
)

func spaRouter(t *testing.T, fsys fs.FS) http.Handler {
	t.Helper()
	router, err := httpapi.NewRouter(t.Context(), httpapi.Dependencies{
		Users:  &usersStub{},
		Tokens: &tokensStub{},
		Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Server: config.Server{TrustedProxies: []string{}},
		Now:    func() time.Time { return time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC) },
		Health: func(context.Context) error { return nil },
		Web:    fsys,
	})
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func spaFixture() fs.FS {
	return fstest.MapFS{
		"index.html":      {Data: []byte("<!doctype html><html><body>spa-shell</body></html>")},
		"assets/app-1.js": {Data: []byte("console.log('spa')")},
		"favicon.svg":     {Data: []byte("<svg/>")},
	}
}

func TestSPAServesIndexForUnknownAndDeepPaths(t *testing.T) {
	router := spaRouter(t, spaFixture())
	for _, path := range []string{"/", "/images", "/tokens", "/assets/missing.js"} {
		w := request(t, router, http.MethodGet, path, "", "")
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s: HTTP=%d want 200", path, w.Code)
		}
		if body := w.Body.String(); !strings.Contains(body, "spa-shell") {
			t.Fatalf("GET %s: body is not the SPA index: %q", path, body)
		}
		if got := w.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
			t.Fatalf("GET %s: content type %q want text/html", path, got)
		}
		if got := w.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("GET %s: cache control %q want no-store", path, got)
		}
	}
}

func TestSPAServesAssetsWithImmutableCache(t *testing.T) {
	router := spaRouter(t, spaFixture())
	w := request(t, router, http.MethodGet, "/assets/app-1.js", "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP=%d want 200", w.Code)
	}
	if body := w.Body.String(); body != "console.log('spa')" {
		t.Fatalf("asset body %q", body)
	}
	if got := w.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Fatalf("cache control %q want immutable", got)
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Fatalf("content type %q want javascript", got)
	}

	root := request(t, router, http.MethodGet, "/favicon.svg", "", "")
	if root.Code != http.StatusOK || root.Body.String() != "<svg/>" {
		t.Fatalf("root static file: HTTP=%d body=%q", root.Code, root.Body.String())
	}
}

func TestSPAReservedPrefixesKeepJSON404(t *testing.T) {
	router := spaRouter(t, spaFixture())
	for _, path := range []string{"/api/nope", "/i/1/2026/a.png", "/t/key.webp", "/api/v1/upload"} {
		w := request(t, router, http.MethodGet, path, "", "")
		if w.Code != http.StatusNotFound {
			t.Fatalf("GET %s: HTTP=%d want 404", path, w.Code)
		}
		expectCode(t, w, http.StatusNotFound, 10001)
	}
}

func TestSPANonGETStaysJSON404(t *testing.T) {
	router := spaRouter(t, spaFixture())
	w := request(t, router, http.MethodPost, "/whatever", "{}", "")
	expectCode(t, w, http.StatusNotFound, 10001)
}

func TestSPAPathTraversalServesIndexNotHostFiles(t *testing.T) {
	router := spaRouter(t, spaFixture())
	for _, path := range []string{"/../server.go", "/..%2fserver.go", "/a/../../router.go"} {
		w := request(t, router, http.MethodGet, path, "", "")
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "spa-shell") {
			t.Fatalf("GET %s: HTTP=%d body=%q want SPA index", path, w.Code, w.Body.String())
		}
	}
}

func TestSPAWithoutWebKeepsJSON404(t *testing.T) {
	router, _, _, _ := routerFixture(t)
	w := request(t, router, http.MethodGet, "/whatever", "", "")
	expectCode(t, w, http.StatusNotFound, 10001)
}
