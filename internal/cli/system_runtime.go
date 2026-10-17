package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/biliblihuorong/imgnest"
	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/release"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/service"
	"gorm.io/gorm"
)

// githubReleases adapts the GitHub release feed to the service interface.
type githubReleases struct{ feed *release.GitHub }

func (g githubReleases) Latest(ctx context.Context) (service.LatestRelease, error) {
	latest, err := g.feed.Latest(ctx)
	if err != nil {
		return service.LatestRelease{}, fmt.Errorf("read latest release: %w", err)
	}
	return service.LatestRelease{Version: latest.Version, URL: latest.URL, PublishedAt: latest.PublishedAt}, nil
}

func newSystemService(ctx context.Context, cfg config.Config) (*service.SystemService, error) {
	var source service.ReleaseSource
	if cfg.Updates.Check {
		source = githubReleases{feed: release.NewGitHub()}
	}
	system, err := service.NewSystemService(ctx, imgnest.Version(), source, time.Now)
	if err != nil {
		return nil, fmt.Errorf("create system service: %w", err)
	}
	return system, nil
}

func newActivityService(ctx context.Context, db *gorm.DB) (*service.ActivityService, error) {
	users, err := repo.NewUserRepository(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("create user repository: %w", err)
	}
	tokens, err := repo.NewTokenRepository(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("create token repository: %w", err)
	}
	activity, err := service.NewActivityService(ctx, users, tokens, time.Now)
	if err != nil {
		return nil, fmt.Errorf("create activity service: %w", err)
	}
	return activity, nil
}
