package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/biliblihuorong/imgnest/internal/backupdir"
	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/migrate"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

// backupScheduleCheck is how often the server looks whether a scheduled
// backup is due; the schedule itself is counted in hours.
const backupScheduleCheck = 5 * time.Minute

// newBackupService writes archives into cfg.Backup.Dir. Each archive carries
// the deployment YAML (when one was given) and the IMGNEST_* environment, so
// a restored database can still unseal its storage credentials.
func newBackupService(ctx context.Context, db *gorm.DB, cfg config.Config, configPath string, logger *slog.Logger) (*service.BackupService, error) {
	dumper, err := repo.NewBackupRepository(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("create backup repository: %w", err)
	}
	settings, err := repo.NewSettingsRepository(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("create settings repository: %w", err)
	}
	store, err := backupdir.New(ctx, cfg.Backup.Dir)
	if err != nil {
		return nil, fmt.Errorf("prepare backup directory: %w", err)
	}
	backups, err := service.NewBackupService(ctx, service.BackupDependencies{
		Dumper: dumper, Store: store, Settings: settings, Now: time.Now, Logger: logger,
		Config: func(context.Context) ([]service.BackupConfigFile, error) {
			return deploymentConfig(configPath, os.Environ())
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create backup service: %w", err)
	}
	return backups, nil
}

func deploymentConfig(configPath string, environ []string) ([]service.BackupConfigFile, error) {
	var files []service.BackupConfigFile
	if configPath != "" {
		content, err := os.ReadFile(configPath) //nolint:gosec // the operator's own --config file
		if err != nil {
			return nil, fmt.Errorf("read configuration file: %w", err)
		}
		files = append(files, service.BackupConfigFile{Name: "imgnest.yaml", Content: content})
	}
	var lines []string
	for _, assignment := range environ {
		if strings.HasPrefix(assignment, "IMGNEST_") {
			lines = append(lines, assignment)
		}
	}
	if len(lines) > 0 {
		slices.Sort(lines)
		files = append(files, service.BackupConfigFile{Name: "imgnest.env", Content: []byte(strings.Join(lines, "\n") + "\n")})
	}
	return files, nil
}

func backupCommand(path *string) *cobra.Command {
	command := &cobra.Command{Use: "backup", Short: "Create or restore database and configuration backups", Args: cobra.NoArgs}
	create := &cobra.Command{Use: "create", Short: "Write a backup archive into backup.dir", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return withDatabase(cmd.Context(), *path, func(db *gorm.DB, cfg config.Config) error {
			backups, err := newBackupService(cmd.Context(), db, cfg, *path, nil)
			if err != nil {
				return err
			}
			file, err := backups.Create(cmd.Context())
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "backup written: %s (%d bytes)\n", file.Name, file.Size)
			return err
		})
	}}
	var confirmed bool
	restore := &cobra.Command{
		Use:   "restore ARCHIVE",
		Short: "Replace every database row with a backup archive (stop the server first)",
		Long: "Replace every database row with the snapshot in ARCHIVE. The database must already be migrated " +
			"by the same ImgNest release that wrote the backup. Configuration files in the archive's config/ " +
			"folder are not applied; copy them back by hand if needed.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirmed {
				return fmt.Errorf("restore replaces all data; rerun with --yes to confirm")
			}
			return withDatabase(cmd.Context(), *path, func(db *gorm.DB, cfg config.Config) error {
				sqlDB, err := db.DB()
				if err != nil {
					return fmt.Errorf("access database: %w", err)
				}
				if err := migrate.Check(cmd.Context(), sqlDB, cfg.Database.Driver); err != nil {
					return fmt.Errorf("check schema: %w", err)
				}
				archive, err := os.Open(args[0])
				if err != nil {
					return fmt.Errorf("open archive: %w", err)
				}
				defer func() { _ = archive.Close() }()
				info, err := archive.Stat()
				if err != nil {
					return fmt.Errorf("open archive: %w", err)
				}
				backups, err := newBackupService(cmd.Context(), db, cfg, *path, nil)
				if err != nil {
					return err
				}
				manifest, err := backups.Restore(cmd.Context(), archive, info.Size())
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "backup from %s restored (%s schema %d)\n", manifest.CreatedAt.Format(time.RFC3339), manifest.Driver, manifest.SchemaVersion)
				return err
			})
		},
	}
	restore.Flags().BoolVar(&confirmed, "yes", false, "confirm replacing all data")
	command.AddCommand(create, restore)
	return command
}
