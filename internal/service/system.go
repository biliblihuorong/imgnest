package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Project links shown on the console's About page.
const (
	RepositoryURL = "https://github.com/biliblihuorong/imgnest"
	DocsURL       = RepositoryURL + "/tree/main/website/docs/guide"
	ReleasesURL   = RepositoryURL + "/releases"
	IssuesURL     = RepositoryURL + "/issues"
)

// LatestRelease is the newest published release reported by a ReleaseSource.
type LatestRelease struct {
	Version     string
	URL         string
	PublishedAt time.Time
}

// ReleaseSource reads the newest published release.
type ReleaseSource interface {
	Latest(ctx context.Context) (LatestRelease, error)
}

// ReleaseView describes the newest published release.
type ReleaseView struct {
	Version     string    `json:"version"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"published_at"`
}

// SystemVersion is the console's About information.
type SystemVersion struct {
	Current string `json:"current"`
	// Latest is null until a check has succeeded.
	Latest          *ReleaseView `json:"latest"`
	UpdateAvailable bool         `json:"update_available"`
	// CheckEnabled is false when the deployment turned update checks off.
	CheckEnabled bool       `json:"check_enabled"`
	CheckedAt    *time.Time `json:"checked_at"`
	// CheckFailed reports that the most recent check could not reach the
	// release feed; Latest then still holds the last good answer.
	CheckFailed   bool   `json:"check_failed"`
	RepositoryURL string `json:"repository_url"`
	DocsURL       string `json:"docs_url"`
	ReleasesURL   string `json:"releases_url"`
	IssuesURL     string `json:"issues_url"`
}

// Update checks are cached; a forced refresh is still spaced out so a
// console cannot turn into a GitHub API client hammering the rate limit.
const (
	updateCheckTTL      = 6 * time.Hour
	updateRetryAfter    = 30 * time.Minute
	updateForceInterval = time.Minute
)

// SystemService reports the running version and whether a newer release exists.
type SystemService struct {
	current string
	source  ReleaseSource
	now     func() time.Time

	mu        sync.Mutex
	latest    *LatestRelease
	checkedAt time.Time
	failed    bool
}

// NewSystemService builds the About service. A nil source turns update
// checks off.
func NewSystemService(ctx context.Context, current string, source ReleaseSource, now func() time.Time) (*SystemService, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("construct system service: %w", err)
	}
	if strings.TrimSpace(current) == "" || now == nil {
		return nil, fmt.Errorf("system service dependencies: %w", ErrInvalidInput)
	}
	return &SystemService{current: strings.TrimSpace(current), source: source, now: now}, nil
}

// Version returns the running version and the cached update check, checking
// again when the cache is stale or refresh asks for it.
func (s *SystemService) Version(ctx context.Context, refresh bool) (SystemVersion, error) {
	if err := ctx.Err(); err != nil {
		return SystemVersion{}, fmt.Errorf("read system version: %w", err)
	}
	view := SystemVersion{
		Current: s.current, CheckEnabled: s.source != nil,
		RepositoryURL: RepositoryURL, DocsURL: DocsURL, ReleasesURL: ReleasesURL, IssuesURL: IssuesURL,
	}
	if s.source == nil {
		return view, nil
	}
	// Holding the lock across the request makes concurrent console loads
	// share one check instead of each calling GitHub.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stale(refresh) {
		latest, err := s.source.Latest(ctx)
		s.checkedAt = s.now().UTC()
		s.failed = err != nil
		if err == nil {
			s.latest = &latest
		}
	}
	if !s.checkedAt.IsZero() {
		at := s.checkedAt
		view.CheckedAt = &at
	}
	view.CheckFailed = s.failed
	if s.latest != nil {
		view.Latest = &ReleaseView{Version: s.latest.Version, URL: s.latest.URL, PublishedAt: s.latest.PublishedAt.UTC()}
		if view.Latest.URL == "" {
			view.Latest.URL = ReleasesURL
		}
		view.UpdateAvailable = CompareVersions(s.latest.Version, s.current) > 0
	}
	return view, nil
}

func (s *SystemService) stale(refresh bool) bool {
	if s.checkedAt.IsZero() {
		return true
	}
	age := s.now().Sub(s.checkedAt)
	if refresh {
		return age >= updateForceInterval
	}
	if s.failed {
		return age >= updateRetryAfter
	}
	return age >= updateCheckTTL
}

// CompareVersions orders two semantic versions (an optional leading v is
// ignored): negative when a < b, zero when equal, positive when a > b. A
// pre-release sorts before its release; unparsable parts compare as zero.
func CompareVersions(a, b string) int {
	coreA, preA := splitVersion(a)
	coreB, preB := splitVersion(b)
	for i := range 3 {
		if coreA[i] != coreB[i] {
			if coreA[i] < coreB[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case preA == preB:
		return 0
	case preA == "":
		return 1
	case preB == "":
		return -1
	case preA < preB:
		return -1
	default:
		return 1
	}
}

func splitVersion(value string) ([3]int, string) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	value, _, _ = strings.Cut(value, "+")
	core, pre, _ := strings.Cut(value, "-")
	var parts [3]int
	for i, part := range strings.SplitN(core, ".", 3) {
		number, err := strconv.Atoi(part)
		if err == nil && number >= 0 {
			parts[i] = number
		}
	}
	return parts, pre
}
