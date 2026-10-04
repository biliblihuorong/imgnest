package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	httpapi "github.com/biliblihuorong/imgnest/internal/http"
	"github.com/biliblihuorong/imgnest/internal/http/lsky"
	"github.com/biliblihuorong/imgnest/internal/migrate"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/biliblihuorong/imgnest/web"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

func serveCommand(path *string) *cobra.Command {
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
			images, closeImages, err := newImageServices(cmd.Context(), db, cfg)
			if err != nil {
				return err
			}
			defer func() {
				if err := closeImages(); err != nil {
					logger.Error("close image runtime", "code", 50002)
				}
			}()
			if err = images.Recover(cmd.Context()); err != nil {
				return fmt.Errorf("recover unfinished image operations: %w", err)
			}
			lskyHandler, err := newLskyHandler(cmd.Context(), db, cfg, users, tokens, images)
			if err != nil {
				return err
			}
			webFS, err := web.DistFS()
			if err != nil {
				return fmt.Errorf("open embedded web app: %w", err)
			}
			handler, err := httpapi.NewRouter(cmd.Context(), httpapi.Dependencies{Users: users, Tokens: tokens, Images: images, ImageOptions: httpapi.ImageOptions{MaxRequestBytes: int64(cfg.Server.MaxRequestMB) << 20, MaxConcurrent: cfg.Server.UploadConcurrency, Timeout: cfg.Server.ProcessingTimeout}, Lsky: lskyHandler, Logger: logger, Server: cfg.Server, Now: time.Now, Health: sqlDB.PingContext, Web: webFS})
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

// newLskyHandler wires the Lsky v1 compatibility layer onto the shared
// services; it owns no business rules of its own.
func newLskyHandler(ctx context.Context, db *gorm.DB, cfg config.Config, users *service.UserService, tokens *service.TokenService, images *service.ImageService) (*lsky.Handler, error) {
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
	albums, err := service.NewAlbumService(ctx, albumsRepo, time.Now)
	if err != nil {
		return nil, fmt.Errorf("create album service: %w", err)
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
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create lsky handler: %w", err)
	}
	return handler, nil
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
