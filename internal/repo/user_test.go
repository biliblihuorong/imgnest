package repo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/migrate"
	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func TestUserPersistenceAndUniqueConstraints(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		users, err := NewUserRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		settings, err := NewSettingsRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		groupID, err := settings.DefaultGroupID(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		created, err := users.CreateUser(t.Context(), testUser("first", "first@example.com", groupID))
		if err != nil {
			t.Fatal(err)
		}
		byEmail, err := users.FindUserByEmail(t.Context(), created.Email)
		if err != nil || byEmail.ID != created.ID || byEmail.PasswordHash != "password-digest" {
			t.Fatalf("read user=%+v, error=%v", byEmail, err)
		}
		byID, err := users.FindUserByID(t.Context(), created.ID)
		if err != nil || byID.GroupID != groupID || byID.CreatedAt.IsZero() {
			t.Fatalf("id lookup=%+v, error=%v", byID, err)
		}
		for _, input := range []model.User{
			testUser("first", "other@example.com", groupID),
			testUser("other", "first@example.com", groupID),
		} {
			if _, err := users.CreateUser(t.Context(), input); !errors.Is(err, model.ErrUserExists) {
				t.Fatalf("duplicate user error=%v, want ErrUserExists", err)
			}
		}
		if _, err := users.FindUserByID(t.Context(), created.ID+99); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("missing user error=%v, want ErrNotFound", err)
		}
	})
}

func TestConcurrentDuplicateRegistration(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		users, settings := repositories(t, db)
		groupID, err := settings.DefaultGroupID(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		for range 2 {
			go func() {
				<-start
				_, err := users.CreateUser(t.Context(), testUser("duplicate", "duplicate@example.com", groupID))
				results <- err
			}()
		}
		close(start)
		var successes, duplicates int
		for range 2 {
			err := <-results
			switch {
			case err == nil:
				successes++
			case errors.Is(err, model.ErrUserExists):
				duplicates++
			default:
				t.Fatalf("unexpected concurrency error: %v", err)
			}
		}
		if successes != 1 || duplicates != 1 {
			t.Fatalf("successes=%d duplicates=%d, want 1 each", successes, duplicates)
		}
	})
}

func TestConcurrentAdminBootstrap(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		users, settings := repositories(t, db)
		groupID, err := settings.DefaultGroupID(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		for _, name := range []string{"adminone", "admintwo"} {
			go func() {
				<-start
				_, err := users.BootstrapAdmin(t.Context(), testUser(name, name+"@example.com", groupID))
				results <- err
			}()
		}
		close(start)
		var successes, forbidden int
		for range 2 {
			err := <-results
			switch {
			case err == nil:
				successes++
			case errors.Is(err, model.ErrForbidden):
				forbidden++
			default:
				t.Fatalf("unexpected bootstrap error: %v", err)
			}
		}
		var admins int64
		if err := db.WithContext(t.Context()).Model(&model.User{}).Where("role = ?", model.UserRoleAdmin).Count(&admins).Error; err != nil {
			t.Fatal(err)
		}
		if successes != 1 || forbidden != 1 || admins != 1 {
			t.Fatalf("success=%d forbidden=%d admin_count=%d, want 1 each", successes, forbidden, admins)
		}
	})
}

func TestAdminBootstrapDoesNotPromoteExistingUser(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		users, settings := repositories(t, db)
		groupID, err := settings.DefaultGroupID(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		input := testUser("ordinary", "ordinary@example.com", groupID)
		created, err := users.CreateUser(t.Context(), input)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := users.BootstrapAdmin(t.Context(), input); !errors.Is(err, model.ErrUserExists) {
			t.Fatalf("existing account error=%v, want ErrUserExists", err)
		}
		found, err := users.FindUserByID(t.Context(), created.ID)
		if err != nil || found.Role != model.UserRoleUser {
			t.Fatalf("ordinary user promoted: role=%q error=%v", found.Role, err)
		}
	})
}

func TestPasswordUpdateRevokesTokensAtomically(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		users, settings := repositories(t, db)
		groupID, err := settings.DefaultGroupID(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		user, err := users.CreateUser(t.Context(), testUser("password", "password@example.com", groupID))
		if err != nil {
			t.Fatal(err)
		}
		tokens, err := NewTokenRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tokens.CreateToken(t.Context(), testToken(user.ID), testGrant(user.PasswordHash)); err != nil {
			t.Fatal(err)
		}
		if err := users.UpdatePasswordAndRevokeTokens(t.Context(), user.ID, user.PasswordHash, "replacement-digest"); err != nil {
			t.Fatal(err)
		}
		found, err := users.FindUserByID(t.Context(), user.ID)
		if err != nil || found.PasswordHash != "replacement-digest" {
			t.Fatalf("updated password missing: %v", err)
		}
		listed, err := tokens.ListTokens(t.Context(), user.ID)
		if err != nil || len(listed) != 0 {
			t.Fatalf("tokens not revoked: count=%d error=%v", len(listed), err)
		}
		if _, err := tokens.CreateToken(t.Context(), testToken(user.ID), testGrant(found.PasswordHash)); err != nil {
			t.Fatal(err)
		}
		if err := db.WithContext(t.Context()).Exec("ALTER TABLE tokens RENAME TO tokens_saved").Error; err != nil {
			t.Fatal(err)
		}
		if err := users.UpdatePasswordAndRevokeTokens(t.Context(), user.ID, found.PasswordHash, "must-roll-back"); err == nil {
			t.Fatal("password update succeeded when revocation could not run")
		}
		found, err = users.FindUserByID(t.Context(), user.ID)
		if err != nil || found.PasswordHash != "replacement-digest" {
			t.Fatalf("password changed despite failed revocation: %v", err)
		}
	})
}

func TestPasswordCASMismatchKeepsPasswordAndTokens(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		for _, scenario := range []string{"mismatch", "disabled"} {
			t.Run(scenario, func(t *testing.T) {
				user, users, tokens := grantFixture(t, db, "cas"+scenario)
				if _, err := tokens.CreateToken(t.Context(), testToken(user.ID), testGrant(user.PasswordHash)); err != nil {
					t.Fatal(err)
				}
				expected := "stale-digest"
				if scenario == "disabled" {
					expected = user.PasswordHash
					if err := db.WithContext(t.Context()).Model(&model.User{}).
						Where("id = ?", user.ID).Update("status", model.UserStatusDisabled).Error; err != nil {
						t.Fatal(err)
					}
				}
				if err := users.UpdatePasswordAndRevokeTokens(
					t.Context(), user.ID, expected, "must-not-replace",
				); !errors.Is(err, model.ErrInvalidCredentials) {
					t.Errorf("CAS %s error=%v, want ErrInvalidCredentials", scenario, err)
				}
				found, err := users.FindUserByID(t.Context(), user.ID)
				if err != nil || found.PasswordHash != user.PasswordHash {
					t.Errorf("failed CAS changed password: error=%v", err)
				}
				remaining, err := tokens.ListTokens(t.Context(), user.ID)
				if err != nil || len(remaining) != 1 {
					t.Errorf("failed CAS revoked tokens: count=%d error=%v", len(remaining), err)
				}
			})
		}
	})
}

func TestConcurrentPasswordCAS(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		user, users, _ := grantFixture(t, db, "concurrentcas")
		start := make(chan struct{})
		results := make(chan error, 2)
		for _, next := range []string{"replacement-one", "replacement-two"} {
			go func() {
				<-start
				results <- users.UpdatePasswordAndRevokeTokens(t.Context(), user.ID, user.PasswordHash, next)
			}()
		}
		close(start)
		var successes, stale int
		for range 2 {
			err := <-results
			switch {
			case err == nil:
				successes++
			case errors.Is(err, model.ErrInvalidCredentials):
				stale++
			default:
				t.Fatal(err)
			}
		}
		if successes != 1 || stale != 1 {
			t.Errorf("CAS successes=%d stale=%d, want 1 each", successes, stale)
		}
	})
}

func TestRepositoryCancelledContext(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		users, settings := repositories(t, db)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := users.FindUserByID(ctx, 1); !errors.Is(err, context.Canceled) {
			t.Fatalf("user lookup error=%v, want canceled", err)
		}
		if _, err := settings.RegistrationEnabled(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("settings lookup error=%v, want canceled", err)
		}
	})
}

func repositories(t *testing.T, db *gorm.DB) (*UserRepository, *SettingsRepository) {
	t.Helper()
	users, err := NewUserRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := NewSettingsRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	return users, settings
}

func testUser(username, email string, groupID uint64) model.User {
	return model.User{
		GroupID: groupID, Username: username, Email: email, PasswordHash: "password-digest",
		Role: model.UserRoleUser, Status: model.UserStatusEnabled,
	}
}

func forEachRepoDatabase(t *testing.T, run func(*testing.T, *gorm.DB)) {
	t.Helper()
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			cfg := config.Database{
				Driver: driver, DSN: filepath.Join(t.TempDir(), "test.db"),
				MaxOpen: 4, MaxIdle: 4, BusyTimeout: 5 * time.Second,
			}
			if driver == "postgres" {
				cfg.DSN = os.Getenv("IMGNEST_TEST_POSTGRES_DSN")
				if cfg.DSN == "" {
					t.Skip("IMGNEST_TEST_POSTGRES_DSN not set")
				}
				cfg.DSN = isolatedRepoPostgres(t, cfg)
			}
			db, err := Open(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := sqlDB.Close(); err != nil {
					t.Error(err)
				}
			})
			if err := migrate.Up(t.Context(), sqlDB, driver); err != nil {
				t.Fatal(err)
			}
			run(t, db)
		})
	}
}

func isolatedRepoPostgres(t *testing.T, cfg config.Database) string {
	t.Helper()
	admin, err := Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	db, err := admin.DB()
	if err != nil {
		t.Fatal(err)
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	schema := "imgnest_repo_" + hex.EncodeToString(random[:])
	if _, err := db.ExecContext(t.Context(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if strings.Contains(cfg.DSN, "://") {
		parsed, err := url.Parse(cfg.DSN)
		if err != nil {
			t.Fatal("invalid PostgreSQL test DSN")
		}
		query := parsed.Query()
		query.Set("search_path", schema)
		parsed.RawQuery = query.Encode()
		return parsed.String()
	}
	return cfg.DSN + " search_path=" + schema
}
