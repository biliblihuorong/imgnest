package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/biliblihuorong/imgnest/extension"
	"github.com/biliblihuorong/imgnest/internal/config"
	httpapi "github.com/biliblihuorong/imgnest/internal/http"
	"github.com/biliblihuorong/imgnest/internal/http/lsky"
	"github.com/biliblihuorong/imgnest/internal/http/ratelimit"
	"github.com/biliblihuorong/imgnest/internal/migrate"
	"github.com/biliblihuorong/imgnest/internal/pathtpl"
	"github.com/biliblihuorong/imgnest/internal/randompool"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

func serveCommand(path *string, plugins []extension.Plugin) *cobra.Command {
	return &cobra.Command{Use: "serve", Short: "Serve the native HTTP API", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return withDatabase(cmd.Context(), *path, func(db *gorm.DB, cfg config.Config) error {
			sqlDB, err := db.DB()
			if err != nil {
				return fmt.Errorf("access database: %w", err)
			}
			if err := migrate.Check(cmd.Context(), sqlDB, cfg.Database.Driver); err != nil {
				return fmt.Errorf("check schema: %w", err)
			}
			users, tokens, err := newServices(cmd.Context(), db)
			if err != nil {
				return err
			}
			logger := slog.New(slog.NewJSONHandler(cmd.OutOrStdout(), nil))
			// One process-local pool is shared by the link service that fills it
			// and the image service that invalidates it.
			randomPool := randompool.NewMemory(time.Now)
			images, albums, closeImages, err := newImageServices(cmd.Context(), db, cfg, randomPool)
			if err != nil {
				return err
			}
			defer func() {
				if err := closeImages(); err != nil {
					logger.Error("close image runtime", "code", 50002)
				}
			}()
			recoverAtStartup(cmd.Context(), images, logger)
			randomLinks, err := newRandomLinkService(cmd.Context(), db, randomPool, logger)
			if err != nil {
				return err
			}
			// One upload limiter spans the native and v1 APIs so a per-minute
			// group quota cannot be doubled by alternating between them.
			uploads := ratelimit.New(time.Now, time.Minute, ratelimit.DefaultCapacity)
			lskyHandler, err := newLskyHandler(cmd.Context(), db, cfg, users, tokens, images, albums, uploads)
			if err != nil {
				return err
			}
			adminService, err := newAdminService(cmd.Context(), db, cfg)
			if err != nil {
				return err
			}
			captchaService, err := newCaptchaService(cmd.Context(), db, cfg)
			if err != nil {
				return err
			}
			webFS, err := frontendDistFS()
			if err != nil {
				return fmt.Errorf("open embedded web app: %w", err)
			}
			handler, err := httpapi.NewRouter(cmd.Context(), httpapi.Dependencies{Users: users, Tokens: tokens, Captcha: captchaService, Images: images, ImageOptions: httpapi.ImageOptions{MaxRequestBytes: int64(cfg.Server.MaxRequestMB) << 20, MaxConcurrent: cfg.Server.UploadConcurrency, Timeout: cfg.Server.ProcessingTimeout, Uploads: uploads}, Albums: albums, RandomLinks: randomLinks, Lsky: lskyHandler, Admin: adminService, AdminImages: images, Logger: logger, Server: cfg.Server, Now: time.Now, Health: sqlDB.PingContext, Web: webFS, Plugins: plugins})
			if err != nil {
				return err
			}
			listener, err := net.Listen("tcp", cfg.Server.Addr)
			if err != nil {
				return fmt.Errorf("listen: %w", err)
			}
			server := &http.Server{Handler: handler, ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout, IdleTimeout: 60 * time.Second, BaseContext: func(net.Listener) context.Context { return cmd.Context() }}
			logger.InfoContext(cmd.Context(), "server listening", "address", listener.Addr().String(), "database_driver", cfg.Database.Driver)
			workerCtx, stopWorker := context.WithCancel(cmd.Context())
			workerDone := make(chan struct{})
			go func() { defer close(workerDone); runImageWorker(workerCtx, images, logger) }()
			result := runServer(cmd.Context(), server, listener, cfg.Server.ShutdownTimeout)
			stopWorker()
			<-workerDone
			return result
		})
	}}
}

// startupRecovery bounds how long a slow storage backend can delay listening.
const startupRecovery = 2 * time.Minute

type imageRecoverer interface {
	Recover(context.Context) error
}

// recoverAtStartup cancels stale uploads and restores before serving. A storage
// failure must not keep the whole site down: unfinished work stays journaled and
// the hourly sweep retries it.
func recoverAtStartup(ctx context.Context, images imageRecoverer, logger *slog.Logger) {
	recoverCtx, cancel := context.WithTimeout(ctx, startupRecovery)
	defer cancel()
	if err := images.Recover(recoverCtx); err != nil && ctx.Err() == nil {
		logger.ErrorContext(ctx, "image recovery pending; retrying in background", "code", 50002)
	}
}

func runImageWorker(ctx context.Context, images *service.ImageService, logger *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			jobCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			err := images.Sweep(jobCtx)
			cancel()
			if err != nil && ctx.Err() == nil {
				logger.ErrorContext(ctx, "image cleanup retry pending", "code", 50002)
			}
		}
	}
}

// newRandomLinkService wires random image links onto the repositories the
// album and image services already use.
func newRandomLinkService(ctx context.Context, db *gorm.DB, pool service.RandomPool, logger *slog.Logger) (*service.RandomLinkService, error) {
	links, err := repo.NewRandomLinkRepository(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("create random link repository: %w", err)
	}
	albums, err := repo.NewAlbumRepository(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("create album repository: %w", err)
	}
	users, err := repo.NewUserRepository(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("create user repository: %w", err)
	}
	storages, err := repo.NewStorageRepository(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("create storage repository: %w", err)
	}
	randomLinks, err := service.NewRandomLinkService(ctx, service.RandomLinkDependencies{
		Links: links, Albums: albums, Users: users, Storages: storages, Pool: pool, Logger: logger,
	})
	if err != nil {
		return nil, fmt.Errorf("create random link service: %w", err)
	}
	return randomLinks, nil
}

// newLskyHandler wires the Lsky v1 compatibility layer onto the shared
// services; it owns no business rules of its own. The album service is shared
// with the native routes so both APIs observe the same ownership rules.
func newLskyHandler(ctx context.Context, db *gorm.DB, cfg config.Config, users *service.UserService, tokens *service.TokenService, images *service.ImageService, albums *service.AlbumService, uploads *ratelimit.Limiter) (*lsky.Handler, error) {
	lskyRepo, err := repo.NewLskyRepository(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("create lsky repository: %w", err)
	}
	albumsRepo, err := repo.NewAlbumRepository(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("create album repository: %w", err)
	}
	policiesRepo, err := repo.NewPolicyRepository(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("create policy repository: %w", err)
	}
	lskyService, err := service.NewLskyService(ctx, service.LskyDependencies{Lsky: lskyRepo, Albums: albumsRepo, Policies: policiesRepo, Now: time.Now})
	if err != nil {
		return nil, fmt.Errorf("create lsky service: %w", err)
	}
	handler, err := lsky.NewHandler(ctx, lsky.Dependencies{
		Users: users, Tokens: tokens, Images: images, Albums: albums, Lsky: lskyService,
		Now: time.Now,
		Options: lsky.Options{
			MaxRequestBytes: int64(cfg.Server.MaxRequestMB) << 20,
			MaxConcurrent:   cfg.Server.UploadConcurrency,
			Timeout:         cfg.Server.ProcessingTimeout,
			TrustedProxies:  cfg.Server.TrustedProxies,
			Uploads:         uploads,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create lsky handler: %w", err)
	}
	return handler, nil
}

// newAdminService wires the management console onto the same repositories the
// other services use; storage creation reuses the provisioning path so cloud
// credentials are sealed and probed exactly like init-storage.
func newAdminService(ctx context.Context, db *gorm.DB, cfg config.Config) (*service.AdminService, error) {
	adminRepo, err := repo.NewAdminRepository(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("create admin repository: %w", err)
	}
	storagesRepo, err := repo.NewStorageRepository(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("create storage repository: %w", err)
	}
	policiesRepo, err := repo.NewPolicyRepository(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("create policy repository: %w", err)
	}
	settingsRepo, err := repo.NewSettingsRepository(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("create settings repository: %w", err)
	}
	drivers, err := newDriverFactory(ctx, cfg.Security.MasterKey)
	if err != nil {
		return nil, err
	}
	provision, err := service.NewProvisionService(ctx, storagesRepo, policiesRepo, drivers, drivers.codec, service.TemplateValidatorFunc(pathtpl.Validate))
	if err != nil {
		_ = drivers.close()
		return nil, fmt.Errorf("create provision service: %w", err)
	}
	adminService, err := service.NewAdminService(ctx, service.AdminDependencies{
		Users: adminRepo, Groups: adminRepo, References: adminRepo,
		Storages: storagesRepo, Policies: policiesRepo, Settings: settingsRepo,
		Provision: provision, Drivers: drivers, Secrets: drivers.codec,
		Templates: service.TemplateValidatorFunc(pathtpl.Validate), Now: time.Now,
	})
	if err != nil {
		_ = drivers.close()
		return nil, fmt.Errorf("create admin service: %w", err)
	}
	return adminService, nil
}

func runServer(ctx context.Context, server *http.Server, listener net.Listener, timeout time.Duration) error {
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	select {
	case err := <-finished:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return errors.Join(fmt.Errorf("shutdown HTTP: %w", err), server.Close())
		}
		if err := <-finished; !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("stop HTTP: %w", err)
		}
		return nil
	}
}
