package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/service"
	"gorm.io/gorm"
)

func withDatabase(ctx context.Context, path string, run func(*gorm.DB, config.Config) error) (result error) {
	cfg, err := config.Load(ctx, path, os.Environ())
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	db, err := repo.Open(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("access database: %w", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			result = errors.Join(result, fmt.Errorf("close database: %w", err))
		}
	}()
	return run(db, cfg)
}

func newServices(ctx context.Context, db *gorm.DB) (*service.UserService, *service.TokenService, error) {
	users, err := repo.NewUserRepository(ctx, db)
	if err != nil {
		return nil, nil, fmt.Errorf("create user repository: %w", err)
	}
	settings, err := repo.NewSettingsRepository(ctx, db)
	if err != nil {
		return nil, nil, fmt.Errorf("create settings repository: %w", err)
	}
	tokens, err := repo.NewTokenRepository(ctx, db)
	if err != nil {
		return nil, nil, fmt.Errorf("create token repository: %w", err)
	}
	u, err := service.NewUserService(ctx, users, settings)
	if err != nil {
		return nil, nil, fmt.Errorf("create user service: %w", err)
	}
	t, err := service.NewTokenService(ctx, tokens, users, settings, time.Now)
	if err != nil {
		return nil, nil, fmt.Errorf("create token service: %w", err)
	}
	return u, t, nil
}
