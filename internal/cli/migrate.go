package cli

import (
	"fmt"

	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/migrate"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

func migrateCommand(path *string) *cobra.Command {
	return &cobra.Command{Use: "migrate", Short: "Apply versioned database migrations", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return withDatabase(cmd.Context(), *path, func(db *gorm.DB, cfg config.Config) error {
			sqlDB, err := db.DB()
			if err != nil {
				return fmt.Errorf("access database: %w", err)
			}
			if err := migrate.Up(cmd.Context(), sqlDB, cfg.Database.Driver); err != nil {
				return fmt.Errorf("migrate database: %w", err)
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), "database migrated"); err != nil {
				return fmt.Errorf("write result: %w", err)
			}
			return nil
		})
	}}
}
