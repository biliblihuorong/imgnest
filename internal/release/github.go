// Package release reads the newest published ImgNest release from GitHub.
package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// LatestURL is the GitHub API endpoint of the newest non-prerelease release.
const LatestURL = "https://api.github.com/repos/biliblihuorong/imgnest/releases/latest"

// ErrUnavailable reports that the release feed could not be read; it carries
// no upstream response data.
var ErrUnavailable = errors.New("release feed unavailable")

// Release is one published version.
type Release struct {
	Version     string
	URL         string
	PublishedAt time.Time
}

// GitHub reads releases through the public, unauthenticated GitHub API.
type GitHub struct {
	client *http.Client
	url    string
}

// NewGitHub builds a bounded client. Unlike the CAPTCHA client it honours
// HTTPS_PROXY, because servers that can only reach GitHub through a proxy
// should still see updates.
func NewGitHub() *GitHub {
	return &GitHub{url: LatestURL, client: &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 5 * time.Second,
			IdleConnTimeout: 30 * time.Second, MaxIdleConns: 2,
		},
	}}
}

// maxBody bounds the release JSON; a release body with long notes is far below it.
const maxBody = 1 << 20

// Latest returns the newest published release.
func (g *GitHub) Latest(ctx context.Context) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.url, nil)
	if err != nil {
		return Release{}, ErrUnavailable
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ImgNest update check")
	resp, err := g.client.Do(req)
	if err != nil {
		return Release{}, ErrUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}
	var body struct {
		TagName     string    `json:"tag_name"`
		HTMLURL     string    `json:"html_url"`
		PublishedAt time.Time `json:"published_at"`
		Draft       bool      `json:"draft"`
		Prerelease  bool      `json:"prerelease"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&body); err != nil {
		return Release{}, ErrUnavailable
	}
	version := strings.TrimPrefix(strings.TrimSpace(body.TagName), "v")
	if version == "" || body.Draft || body.Prerelease {
		return Release{}, ErrUnavailable
	}
	link := body.HTMLURL
	if !strings.HasPrefix(link, "https://github.com/") {
		link = ""
	}
	return Release{Version: version, URL: link, PublishedAt: body.PublishedAt.UTC()}, nil
}
