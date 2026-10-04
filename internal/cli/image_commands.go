package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/migrate"
	"github.com/biliblihuorong/imgnest/internal/pathtpl"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

func withProvision(ctx context.Context, path string, run func(*service.ProvisionService, *repo.StorageRepository, *repo.SettingsRepository) error) error {
	return withDatabase(ctx, path, func(db *gorm.DB, cfg config.Config) (result error) {
		sqlDB, err := db.DB()
		if err != nil {
			return fmt.Errorf("access database: %w", err)
		}
		if err = migrate.Check(ctx, sqlDB, cfg.Database.Driver); err != nil {
			return err
		}
		stores, err := repo.NewStorageRepository(ctx, db)
		if err != nil {
			return err
		}
		policies, err := repo.NewPolicyRepository(ctx, db)
		if err != nil {
			return err
		}
		settings, err := repo.NewSettingsRepository(ctx, db)
		if err != nil {
			return err
		}
		drivers, err := newDriverFactory(ctx, cfg.Security.MasterKey)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, drivers.close()) }()
		setup, err := service.NewProvisionService(ctx, stores, policies, drivers, drivers.codec, service.TemplateValidatorFunc(pathtpl.Validate))
		if err != nil {
			return err
		}
		return run(setup, stores, settings)
	})
}

func localCommand(path *string) *cobra.Command {
	var directory, baseURL, name string
	var groupID uint64
	cmd := &cobra.Command{Use: "init-local", Short: "Create a local storage and default upload policy", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return withProvision(cmd.Context(), *path, func(setup *service.ProvisionService, stores *repo.StorageRepository, settings *repo.SettingsRepository) error {
			group := groupID
			if group == 0 {
				var err error
				group, err = settings.DefaultGroupID(cmd.Context())
				if err != nil {
					return err
				}
			}
			config, err := json.Marshal(localConfig{Root: directory})
			if err != nil {
				return service.ErrInvalidInput
			}
			value, err := setup.CreateStorage(cmd.Context(), service.StorageInput{Name: name, Driver: "local", BaseURL: baseURL, Config: config})
			if err != nil {
				return err
			}
			value.BaseURL = strings.TrimRight(baseURL, "/") + "/i/" + strconv.FormatUint(value.ID, 10)
			if _, err = stores.SetBaseURL(cmd.Context(), value.ID, value.BaseURL); err != nil {
				return err
			}
			policy, err := service.DefaultPolicy(cmd.Context(), value.ID, name+" default")
			if err != nil {
				return err
			}
			policy, err = setup.CreatePolicy(cmd.Context(), policy, group, true)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
				Storage  service.StorageView `json:"storage"`
				PolicyID uint64              `json:"policy_id"`
			}{value, policy.ID})
		})
	}}
	cmd.Flags().StringVar(&directory, "root", "data/images", "object directory (internal ImgNest format)")
	cmd.Flags().StringVar(&baseURL, "base-url", "http://localhost:8080", "public origin; /i/<storageID> is appended")
	cmd.Flags().StringVar(&name, "name", "local", "storage name")
	cmd.Flags().Uint64Var(&groupID, "group-id", 0, "group to bind (0 selects configured default)")
	return cmd
}

func storageCommand(path *string) *cobra.Command {
	return &cobra.Command{Use: "init-storage", Short: "Create a tested storage from JSON on stdin (cloud credentials require master key)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		var input service.StorageInput
		if err := readSetupJSON(cmd.Context(), cmd.InOrStdin(), &input); err != nil {
			return err
		}
		return withProvision(cmd.Context(), *path, func(setup *service.ProvisionService, _ *repo.StorageRepository, _ *repo.SettingsRepository) error {
			value, err := setup.CreateStorage(cmd.Context(), input)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(value)
		})
	}}
}

func policyCommand(path *string) *cobra.Command {
	var storageID, groupID uint64
	var name string
	var useStdin, makeDefault bool
	cmd := &cobra.Command{Use: "init-policy", Short: "Create an upload policy; optional JSON stdin overrides defaults", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		policy, err := service.DefaultPolicy(cmd.Context(), storageID, name)
		if err != nil {
			return err
		}
		if useStdin {
			if err = readSetupJSON(cmd.Context(), cmd.InOrStdin(), &policy); err != nil {
				return err
			}
		}
		return withProvision(cmd.Context(), *path, func(setup *service.ProvisionService, _ *repo.StorageRepository, settings *repo.SettingsRepository) error {
			group := groupID
			if group == 0 {
				var err error
				group, err = settings.DefaultGroupID(cmd.Context())
				if err != nil {
					return err
				}
			}
			value, err := setup.CreatePolicy(cmd.Context(), policy, group, makeDefault)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(value)
		})
	}}
	cmd.Flags().Uint64Var(&storageID, "storage-id", 0, "storage ID")
	cmd.Flags().Uint64Var(&groupID, "group-id", 0, "group to bind (0 selects configured default)")
	cmd.Flags().StringVar(&name, "name", "default", "policy name")
	cmd.Flags().BoolVar(&useStdin, "stdin", false, "read processing/path overrides as JSON")
	cmd.Flags().BoolVar(&makeDefault, "default", true, "set as this group's default policy")
	return cmd
}

func readSetupJSON(ctx context.Context, input io.Reader, target any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	type result struct {
		data []byte
		err  error
	}
	ready := make(chan result, 1)
	// ponytail: a canceled one-shot command exits; an arbitrary stdin Reader has no cancellation API.
	go func() { data, err := io.ReadAll(io.LimitReader(input, (64<<10)+1)); ready <- result{data, err} }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case value := <-ready:
		if err := ctx.Err(); err != nil {
			return err
		}
		if value.err != nil || len(value.data) > 64<<10 {
			return service.ErrInvalidInput
		}
		decoder := json.NewDecoder(strings.NewReader(string(value.data)))
		decoder.DisallowUnknownFields()
		if decoder.Decode(target) != nil {
			return service.ErrInvalidInput
		}
		if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
			return service.ErrInvalidInput
		}
		return nil
	}
}
