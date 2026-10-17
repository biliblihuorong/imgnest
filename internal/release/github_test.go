package release

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func serve(t *testing.T, status int, body string) *GitHub {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	g := NewGitHub()
	g.url = server.URL
	return g
}

func TestLatestParsesRelease(t *testing.T) {
	g := serve(t, 200, `{"tag_name":"v1.2.3","html_url":"https://github.com/biliblihuorong/imgnest/releases/tag/v1.2.3","published_at":"2026-10-01T08:00:00Z"}`)
	got, err := g.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := Release{Version: "1.2.3", URL: "https://github.com/biliblihuorong/imgnest/releases/tag/v1.2.3", PublishedAt: time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestLatestDropsForeignLink(t *testing.T) {
	g := serve(t, 200, `{"tag_name":"1.0.0","html_url":"https://evil.example/x"}`)
	got, err := g.Latest(context.Background())
	if err != nil || got.URL != "" || got.Version != "1.0.0" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestLatestRejectsBadResponses(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
	}{
		"status":     {404, `{"message":"Not Found"}`},
		"json":       {200, `{`},
		"empty tag":  {200, `{"tag_name":""}`},
		"prerelease": {200, `{"tag_name":"v2.0.0-rc.1","prerelease":true}`},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := serve(t, tc.status, tc.body).Latest(context.Background()); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("got %v", err)
			}
		})
	}
}
