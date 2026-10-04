package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/migrate"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

func adminCommand(path *string, reset bool) *cobra.Command {
	var username, email string
	name, description := "init-admin", "Initialize the first administrator (password from stdin)"
	if reset {
		name, description = "reset-password", "Reset a user's password and revoke tokens (password from stdin)"
	}
	command := &cobra.Command{Use: name, Short: description, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return withDatabase(cmd.Context(), *path, func(db *gorm.DB, cfg config.Config) error {
			sqlDB, err := db.DB()
			if err != nil {
				return fmt.Errorf("access database: %w", err)
			}
			if err := migrate.Check(cmd.Context(), sqlDB, cfg.Database.Driver); err != nil {
				return fmt.Errorf("check schema: %w", err)
			}
			password, err := readPassword(cmd.Context(), cmd.InOrStdin())
			if err != nil {
				return err
			}
			if len(password) < 12 || len(password) > 72 {
				return service.ErrInvalidInput
			}
			users, _, err := newServices(cmd.Context(), db)
			if err != nil {
				return err
			}
			if reset {
				err = users.ResetPassword(cmd.Context(), email, password)
			} else {
				_, err = users.InitAdmin(cmd.Context(), service.RegisterInput{Username: username, Email: email, Password: password})
			}
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), name+" completed"); err != nil {
				return fmt.Errorf("write result: %w", err)
			}
			return nil
		})
	}}
	command.Flags().StringVar(&email, "email", "", "user email")
	if !reset {
		command.Flags().StringVar(&username, "username", "", "administrator username")
	}
	return command
}

func readPassword(ctx context.Context, input io.Reader) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	type result struct {
		line string
		err  error
	}
	ready := make(chan result, 1)
	// ponytail: an arbitrary Reader cannot be interrupted; a canceled one-shot CLI exits while its read finishes.
	go func() {
		line, err := bufio.NewReader(io.LimitReader(input, 74)).ReadString('\n')
		ready <- result{line, err}
	}()
	select {
	case <-ctx.Done():
		return "", fmt.Errorf("read password: %w", ctx.Err())
	case value := <-ready:
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		if value.err != nil && !errors.Is(value.err, io.EOF) {
			return "", errors.New("read password: input unavailable")
		}
		return strings.TrimSuffix(strings.TrimSuffix(value.line, "\n"), "\r"), nil
	}
}
