package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/exif"
	"github.com/biliblihuorong/imgnest/internal/imaging"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/pathtpl"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/secret"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/biliblihuorong/imgnest/internal/storage"
	"github.com/biliblihuorong/imgnest/internal/thumbcache"
	"gorm.io/gorm"
)

type localConfig struct {
	Root string `json:"root"`
}
type cloudConfig struct {
	Endpoint        string `json:"endpoint"`
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	SessionToken    string `json:"session_token"`
	UsePathStyle    bool   `json:"use_path_style"`
}

type driverFactory struct {
	mu      sync.Mutex
	codec   *secret.Codec
	drivers map[string]storage.Driver
}

func newDriverFactory(ctx context.Context, key string) (*driverFactory, error) {
	codec, err := secret.NewCodec(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("configure storage encryption: %w", err)
	}
	return &driverFactory{codec: codec, drivers: map[string]storage.Driver{}}, nil
}
func (f *driverFactory) DriverFor(ctx context.Context, backend model.Storage) (storage.Driver, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cacheKey := fmt.Sprintf("%d/%s/%x", backend.ID, backend.Driver, sha256.Sum256(backend.Config))
	f.mu.Lock()
	defer f.mu.Unlock()
	if driver := f.drivers[cacheKey]; driver != nil {
		return driver, nil
	}
	var driver storage.Driver
	switch backend.Driver {
	case "local":
		var cfg localConfig
		if json.Unmarshal(backend.Config, &cfg) != nil || cfg.Root == "" {
			return nil, service.ErrInvalidInput
		}
		local, err := storage.NewLocal(ctx, cfg.Root)
		if err != nil {
			return nil, service.ErrStorage
		}
		driver = local
	case "s3":
		plain, err := f.codec.Open(ctx, "s3", backend.Config)
		if err != nil {
			return nil, service.ErrStorage
		}
		var cfg cloudConfig
		if json.Unmarshal(plain, &cfg) != nil {
			return nil, service.ErrStorage
		}
		s3, err := storage.NewS3(ctx, storage.S3Config{Endpoint: cfg.Endpoint, Region: cfg.Region, Bucket: cfg.Bucket, AccessKeyID: cfg.AccessKeyID, SecretAccessKey: cfg.SecretAccessKey, SessionToken: cfg.SessionToken, UsePathStyle: cfg.UsePathStyle})
		if err != nil {
			return nil, service.ErrStorage
		}
		driver = s3
	default:
		return nil, service.ErrInvalidInput
	}
	f.drivers[cacheKey] = driver
	return driver, nil
}
func (f *driverFactory) close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result error
	for _, driver := range f.drivers {
		if closer, ok := driver.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				result = service.ErrStorage
			}
		}
	}
	return result
}

// newImageServices wires the image runtime together with the album repository
// it needs for album assignment and the album service sharing the same repos.
// randomPool, when set, is told which albums' random-link candidates changed.
func newImageServices(ctx context.Context, db *gorm.DB, cfg config.Config, randomPool service.RandomPoolInvalidator, hooks imageHooks) (*service.ImageService, *service.AlbumService, func() error, error) {
	images, err := repo.NewImageRepository(ctx, db)
	if err != nil {
		return nil, nil, nil, err
	}
	albumsRepo, err := repo.NewAlbumRepository(ctx, db)
	if err != nil {
		return nil, nil, nil, err
	}
	policies, err := repo.NewPolicyRepository(ctx, db)
	if err != nil {
		return nil, nil, nil, err
	}
	stores, err := repo.NewStorageRepository(ctx, db)
	if err != nil {
		return nil, nil, nil, err
	}
	users, err := repo.NewUserRepository(ctx, db)
	if err != nil {
		return nil, nil, nil, err
	}
	tokens, err := repo.NewTokenRepository(ctx, db)
	if err != nil {
		return nil, nil, nil, err
	}
	settings, err := repo.NewSettingsRepository(ctx, db)
	if err != nil {
		return nil, nil, nil, err
	}
	drivers, err := newDriverFactory(ctx, cfg.Security.MasterKey)
	if err != nil {
		return nil, nil, nil, err
	}
	cache, err := thumbcache.New(ctx, cfg.Images.ThumbCache)
	if err != nil {
		_ = drivers.close()
		return nil, nil, nil, service.ErrStorage
	}
	cleanup := func() error {
		cacheErr := cache.Close()
		driverErr := drivers.close()
		if cacheErr != nil || driverErr != nil {
			return service.ErrStorage
		}
		return nil
	}
	albums, err := service.NewAlbumService(ctx, albumsRepo, images, time.Now)
	if err != nil {
		_ = cleanup()
		return nil, nil, nil, err
	}
	processor, err := imaging.NewProcessor(ctx, int64(cfg.Server.MaxPixels), runtime.GOMAXPROCS(0))
	if err != nil {
		_ = cleanup()
		return nil, nil, nil, service.ErrProcessing
	}
	metadata, err := exif.NewProcessor(ctx)
	if err != nil {
		_ = cleanup()
		return nil, nil, nil, service.ErrProcessing
	}
	svc, err := service.NewImageService(ctx, service.ImageDependencies{Images: images, Policies: policies, Storages: stores, Users: users, Tokens: tokens, Drivers: drivers, Paths: service.PathFunctions{BuildPath: pathtpl.Build, CleanPath: pathtpl.Sanitize}, Imaging: processor, Extractor: metadata, Scrubber: metadata, Cache: cache, Settings: settings, Albums: albumsRepo, RandomPool: randomPool, Events: hooks.Events, Inspectors: hooks.Inspectors, TrashPolicies: hooks.Trash, Display: hooks.Display, Now: time.Now, MaxFileBytes: int64(cfg.Server.MaxUploadMB) << 20})
	if err != nil {
		_ = cleanup()
		return nil, nil, nil, err
	}
	return svc, albums, cleanup, nil
}
