package service_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/migrate"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/service"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const testPassword = "correct-password-123"

type authFixture struct {
	db       *gorm.DB
	users    *repo.UserRepository
	settings *repo.SettingsRepository
	tokens   *repo.TokenRepository
	service  *service.UserService
}

func forDatabases(t *testing.T, run func(*testing.T, *authFixture)) {
	t.Helper()
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			run(t, newAuthFixture(t, driver))
		})
	}
}

func newAuthFixture(t *testing.T, driver string) *authFixture {
	t.Helper()
	cfg, err := config.Load(t.Context(), "", []string{})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database.Driver = driver
	cfg.Database.DSN = filepath.Join(t.TempDir(), "auth.db")
	if driver == "postgres" {
		cfg.Database.MaxOpen = 5
		cfg.Database.MaxIdle = 5
		dsn := os.Getenv("IMGNEST_TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("IMGNEST_TEST_POSTGRES_DSN is not set")
		}
		cfg.Database.DSN = dsn
		admin, err := repo.Open(t.Context(), cfg.Database)
		if err != nil {
			t.Fatal(err)
		}
		adminDB, err := admin.DB()
		if err != nil {
			t.Fatal(err)
		}
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			t.Fatal(err)
		}
		schema := "service_" + hex.EncodeToString(random[:])
		if _, err := adminDB.ExecContext(t.Context(), "CREATE SCHEMA "+schema); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := adminDB.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
				t.Error(err)
			}
			if err := adminDB.Close(); err != nil {
				t.Error(err)
			}
		})
		if strings.Contains(dsn, "://") {
			parsed, err := url.Parse(dsn)
			if err != nil {
				t.Fatal("invalid test database URL")
			}
			query := parsed.Query()
			query.Set("search_path", schema)
			parsed.RawQuery = query.Encode()
			cfg.Database.DSN = parsed.String()
		} else {
			cfg.Database.DSN = dsn + " search_path=" + schema
		}
	}
	db, err := repo.Open(t.Context(), cfg.Database)
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
	users, err := repo.NewUserRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := repo.NewSettingsRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := repo.NewTokenRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	usersService, err := service.NewUserService(t.Context(), users, settings)
	if err != nil {
		t.Fatal(err)
	}
	return &authFixture{db: db, users: users, settings: settings, tokens: tokens, service: usersService}
}

func enableRegistration(t *testing.T, fixture *authFixture) {
	t.Helper()
	if err := fixture.db.WithContext(t.Context()).Model(&model.Setting{}).
		Where("key = ?", "registration_enabled").Update("value", "true").Error; err != nil {
		t.Fatal(err)
	}
}

func registerUser(t *testing.T, fixture *authFixture, name string) service.UserView {
	t.Helper()
	enableRegistration(t, fixture)
	user, err := fixture.service.Register(t.Context(), service.RegisterInput{
		Username: name, Email: name + "@example.com", Password: testPassword,
	})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func createStoredToken(t *testing.T, fixture *authFixture, userID uint64) model.Token {
	t.Helper()
	user, err := fixture.users.FindUserByID(t.Context(), userID)
	if err != nil {
		t.Fatal(err)
	}
	var digest [32]byte
	if _, err := rand.Read(digest[:]); err != nil {
		t.Fatal(err)
	}
	token, err := fixture.tokens.CreateToken(t.Context(), model.Token{
		UserID: userID, Name: "test", Kind: "api",
		TokenHash: hex.EncodeToString(digest[:]), Abilities: []string{"*"},
	}, model.TokenGrant{ExpectedPasswordHash: user.PasswordHash, At: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestRegisterDisabled(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		_, err := fixture.service.Register(t.Context(), service.RegisterInput{
			Username: "alice", Email: "alice@example.com", Password: testPassword,
		})
		if !errors.Is(err, service.ErrRegistrationDisabled) {
			t.Fatalf("registration error=%v, want disabled", err)
		}
		// Migration 0003 adds the never-authenticating guest anchor row with
		// id 0; registrations must not write any real account on top of it.
		var count int64
		if err := fixture.db.WithContext(t.Context()).Model(&model.User{}).Where("id > 0").Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("disabled registration wrote %d users", count)
		}
	})
}

func TestRegisterUsesDefaultGroupAndUserRole(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		enableRegistration(t, fixture)
		user, err := fixture.service.Register(t.Context(), service.RegisterInput{
			Username: "alice", Email: " Alice@Example.COM ", Password: testPassword,
		})
		if err != nil {
			t.Fatal(err)
		}
		groupID, err := fixture.settings.DefaultGroupID(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if user.GroupID != groupID || user.Role != "user" || user.Status != "enabled" {
			t.Fatalf("registration returned wrong group/role/status: %+v", user)
		}
		if user.ID == 0 || user.Email != "alice@example.com" {
			t.Fatalf("registration returned wrong ID or normalized email: %+v", user)
		}
		stored, err := fixture.users.FindUserByID(t.Context(), user.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte(testPassword)); err != nil {
			t.Fatal("stored password does not verify")
		}
		cost, err := bcrypt.Cost([]byte(stored.PasswordHash))
		if err != nil || cost != 12 {
			t.Fatalf("bcrypt cost=%d, err=%v; want 12", cost, err)
		}
		body, err := json.Marshal(user)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "hash") || strings.Contains(string(body), testPassword) {
			t.Fatal("user DTO exposed a password or hash")
		}
		if !strings.Contains(string(body), "\"group_id\"") || !strings.Contains(string(body), "\"created_at\"") {
			t.Fatal("user DTO does not use snake_case fields")
		}
	})
}

func TestRegisterInputValidation(t *testing.T) {
	cases := []struct {
		name     string
		username string
		email    string
		wantErr  error
	}{
		{name: "short username", username: "ab", email: "alice@example.com", wantErr: service.ErrInvalidInput},
		{name: "minimum runes", username: "图床人", email: "alice@example.com"},
		{name: "maximum runes", username: strings.Repeat("图", 64), email: "alice@example.com"},
		{
			name: "long username", username: strings.Repeat("图", 65),
			email: "alice@example.com", wantErr: service.ErrInvalidInput,
		},
		{name: "invalid email", username: "alice", email: "missing-domain", wantErr: service.ErrInvalidInput},
		{name: "display name", username: "alice", email: "Alice <alice@example.com>", wantErr: service.ErrInvalidInput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newAuthFixture(t, "sqlite")
			enableRegistration(t, fixture)
			_, err := fixture.service.Register(t.Context(), service.RegisterInput{
				Username: tc.username, Email: tc.email, Password: testPassword,
			})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("registration error=%v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestPasswordByteLimits(t *testing.T) {
	cases := []struct {
		name     string
		password string
		wantErr  error
	}{
		{name: "11 bytes", password: strings.Repeat("a", 11), wantErr: service.ErrInvalidInput},
		{name: "12 bytes", password: strings.Repeat("a", 12)},
		{name: "72 bytes", password: strings.Repeat("a", 72)},
		{name: "73 bytes", password: strings.Repeat("a", 73), wantErr: service.ErrInvalidInput},
		{name: "12 multibyte bytes", password: strings.Repeat("图", 4)},
		{name: "75 multibyte bytes", password: strings.Repeat("图", 25), wantErr: service.ErrInvalidInput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newAuthFixture(t, "sqlite")
			enableRegistration(t, fixture)
			_, err := fixture.service.Register(t.Context(), service.RegisterInput{
				Username: "alice", Email: "alice@example.com", Password: tc.password,
			})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("registration error=%v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestVerifyWrongCredentials(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		user := registerUser(t, fixture, "alice")
		for _, email := range []string{"missing@example.com", "alice@example.com", "Alice <alice@example.com>"} {
			_, err := fixture.service.VerifyCredentials(t.Context(), email, "wrong-password")
			if !errors.Is(err, service.ErrInvalidCredentials) {
				t.Fatalf("wrong credentials error=%v, want invalid credentials", err)
			}
		}
		verified, err := fixture.service.VerifyCredentials(t.Context(), " Alice@Example.COM ", testPassword)
		if err != nil || verified.User.ID != user.ID {
			t.Fatalf("valid credentials failed: ID=%d, err=%v", verified.User.ID, err)
		}
		if err := fixture.db.WithContext(t.Context()).Model(&model.User{}).
			Where("id = ?", user.ID).Update("status", "disabled").Error; err != nil {
			t.Fatal(err)
		}
		_, err = fixture.service.VerifyCredentials(t.Context(), user.Email, testPassword)
		if !errors.Is(err, service.ErrInvalidCredentials) {
			t.Fatalf("disabled user credentials error=%v", err)
		}
	})
}

func TestCredentialsRejectOverlongPassword(t *testing.T) {
	fixture := newAuthFixture(t, "sqlite")
	enableRegistration(t, fixture)
	password := strings.Repeat("a", 72)
	user, err := fixture.service.Register(t.Context(), service.RegisterInput{
		Username: "alice", Email: "alice@example.com", Password: password,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.VerifyCredentials(t.Context(), user.Email, password); err != nil {
		t.Fatal("exact maximum-length password did not verify")
	}
	_, err = fixture.service.VerifyCredentials(t.Context(), user.Email, password+"b")
	if !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("overlong password was accepted: %v", err)
	}
}

func TestChangePasswordRejectsOverlongCurrentPassword(t *testing.T) {
	fixture := newAuthFixture(t, "sqlite")
	enableRegistration(t, fixture)
	password := strings.Repeat("a", 72)
	user, err := fixture.service.Register(t.Context(), service.RegisterInput{
		Username: "alice", Email: "alice@example.com", Password: password,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.service.ChangePassword(
		t.Context(),
		user.ID,
		password+"b",
		"next-password-123",
	)
	if !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("overlong current password was accepted: %v", err)
	}
	if _, err := fixture.service.VerifyCredentials(t.Context(), user.Email, password); err != nil {
		t.Fatal("rejected password change modified the original credential")
	}
}

func createLegacyUser(t *testing.T, fixture *authFixture) model.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("oldpass8"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	groupID, err := fixture.settings.DefaultGroupID(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	user, err := fixture.users.CreateUser(t.Context(), model.User{
		Username: "legacy", Email: "legacy@example.com", GroupID: groupID,
		PasswordHash: string(hash), Role: "user", Status: "enabled",
	})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func TestVerifyLegacyShortPassword(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		user := createLegacyUser(t, fixture)
		verified, err := fixture.service.VerifyCredentials(t.Context(), user.Email, "oldpass8")
		if err != nil || verified.User.ID != user.ID {
			t.Fatalf("legacy bcrypt credentials were rejected: ID=%d, err=%v", verified.User.ID, err)
		}
		_, err = fixture.service.VerifyCredentials(t.Context(), user.Email, "wrongold")
		if !errors.Is(err, service.ErrInvalidCredentials) {
			t.Fatalf("wrong short legacy password authenticated: %v", err)
		}
	})
}

func TestChangePasswordWithLegacyShortCurrentPassword(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		user := createLegacyUser(t, fixture)
		createStoredToken(t, fixture, user.ID)
		if err := fixture.service.ChangePassword(
			t.Context(),
			user.ID,
			"oldpass8",
			"next-password-123",
		); err != nil {
			t.Fatalf("legacy current password was rejected: %v", err)
		}
		if _, err := fixture.service.VerifyCredentials(t.Context(), user.Email, "next-password-123"); err != nil {
			t.Fatal("new password was not stored for legacy account")
		}
		tokens, err := fixture.tokens.ListTokens(t.Context(), user.ID)
		if err != nil || len(tokens) != 0 {
			t.Fatalf("legacy password change retained tokens: count=%d, err=%v", len(tokens), err)
		}
	})
}

func TestInitAdminRequiresNoExistingAdmin(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		admin, err := fixture.service.InitAdmin(t.Context(), service.RegisterInput{
			Username: "admin", Email: "admin@example.com", Password: testPassword,
		})
		if err != nil || admin.Role != "admin" {
			t.Fatalf("explicit admin initialization failed: role=%q, err=%v", admin.Role, err)
		}
		_, err = fixture.service.InitAdmin(t.Context(), service.RegisterInput{
			Username: "other", Email: "other@example.com", Password: testPassword,
		})
		if !errors.Is(err, service.ErrForbidden) {
			t.Fatalf("second admin initialization error=%v, want forbidden", err)
		}
	})
}

func TestInitAdminCannotPromoteExistingUser(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		user := registerUser(t, fixture, "alice")
		_, err := fixture.service.InitAdmin(t.Context(), service.RegisterInput{
			Username: user.Username, Email: user.Email, Password: testPassword,
		})
		if !errors.Is(err, service.ErrUserExists) {
			t.Fatalf("initializing an existing account error=%v, want user exists", err)
		}
		stored, err := fixture.users.FindUserByID(t.Context(), user.ID)
		if err != nil || stored.Role != "user" {
			t.Fatalf("existing account changed role: role=%q, err=%v", stored.Role, err)
		}
	})
}

func TestChangePasswordRevokesAllTokens(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		user := registerUser(t, fixture, "alice")
		createStoredToken(t, fixture, user.ID)
		createStoredToken(t, fixture, user.ID)
		err := fixture.service.ChangePassword(
			t.Context(),
			user.ID,
			"incorrect-password",
			"next-password-123",
		)
		if !errors.Is(err, service.ErrInvalidCredentials) {
			t.Fatalf("wrong current password error=%v", err)
		}
		before, err := fixture.tokens.ListTokens(t.Context(), user.ID)
		if err != nil || len(before) != 2 {
			t.Fatalf("wrong password revoked tokens: count=%d, err=%v", len(before), err)
		}
		if err := fixture.service.ChangePassword(
			t.Context(),
			user.ID,
			testPassword,
			"next-password-123",
		); err != nil {
			t.Fatal(err)
		}
		after, err := fixture.tokens.ListTokens(t.Context(), user.ID)
		if err != nil || len(after) != 0 {
			t.Fatalf("changed password retained tokens: count=%d, err=%v", len(after), err)
		}
		_, err = fixture.service.VerifyCredentials(t.Context(), user.Email, testPassword)
		if !errors.Is(err, service.ErrInvalidCredentials) {
			t.Fatalf("old password still valid: %v", err)
		}
		if _, err := fixture.service.VerifyCredentials(t.Context(), user.Email, "next-password-123"); err != nil {
			t.Fatal("new password is invalid")
		}
	})
}

func TestResetPasswordRevokesAllTokens(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		user := registerUser(t, fixture, "alice")
		createStoredToken(t, fixture, user.ID)
		if err := fixture.service.ResetPassword(t.Context(), " ALICE@EXAMPLE.COM ", "reset-password-123"); err != nil {
			t.Fatal(err)
		}
		tokens, err := fixture.tokens.ListTokens(t.Context(), user.ID)
		if err != nil || len(tokens) != 0 {
			t.Fatalf("password reset retained tokens: count=%d, err=%v", len(tokens), err)
		}
		if _, err := fixture.service.VerifyCredentials(t.Context(), user.Email, "reset-password-123"); err != nil {
			t.Fatal("reset password is invalid")
		}
	})
}

type passwordUpdateBarrier struct {
	service.UserRepository
	entered chan struct{}
	release chan struct{}
}

func (barrier passwordUpdateBarrier) UpdatePasswordAndRevokeTokens(
	ctx context.Context,
	userID uint64,
	expectedHash, nextHash string,
) error {
	close(barrier.entered)
	select {
	case <-barrier.release:
		return barrier.UserRepository.UpdatePasswordAndRevokeTokens(
			ctx,
			userID,
			expectedHash,
			nextHash,
		)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestStalePasswordChangeCannotOverwriteReset(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		user := registerUser(t, fixture, "alice")
		entered := make(chan struct{})
		release := make(chan struct{})
		t.Cleanup(func() {
			select {
			case <-release:
			default:
				close(release)
			}
		})
		blocked, err := service.NewUserService(t.Context(), passwordUpdateBarrier{
			UserRepository: fixture.users, entered: entered, release: release,
		}, fixture.settings)
		if err != nil {
			t.Fatal(err)
		}
		result := make(chan error, 1)
		go func() {
			result <- blocked.ChangePassword(
				t.Context(),
				user.ID,
				testPassword,
				"stale-password-123",
			)
		}()
		select {
		case <-entered:
		case err := <-result:
			t.Fatalf("password change never reached the conditional update: %v", err)
		}
		if err := fixture.service.ResetPassword(t.Context(), user.Email, "reset-password-123"); err != nil {
			t.Fatal(err)
		}
		close(release)
		if err := <-result; !errors.Is(err, service.ErrInvalidCredentials) {
			t.Fatalf("stale password change was accepted after reset: %v", err)
		}
		if _, err := fixture.service.VerifyCredentials(t.Context(), user.Email, "reset-password-123"); err != nil {
			t.Fatal("stale change overwrote the completed password reset")
		}
	})
}

func TestConcurrentDuplicateRegistration(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		enableRegistration(t, fixture)
		cases := []struct {
			name   string
			inputs []service.RegisterInput
		}{
			{name: "duplicate email", inputs: []service.RegisterInput{
				{Username: "alice", Email: "same@example.com", Password: testPassword},
				{Username: "other", Email: " SAME@EXAMPLE.COM ", Password: testPassword},
			}},
			{name: "duplicate username", inputs: []service.RegisterInput{
				{Username: "same", Email: "first@example.com", Password: testPassword},
				{Username: "same", Email: "second@example.com", Password: testPassword},
			}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				start := make(chan struct{})
				results := make(chan error, 2)
				var workers sync.WaitGroup
				for _, input := range tc.inputs {
					workers.Go(func() {
						<-start
						_, err := fixture.service.Register(t.Context(), input)
						results <- err
					})
				}
				close(start)
				workers.Wait()
				close(results)
				var succeeded, duplicate int
				for err := range results {
					switch {
					case err == nil:
						succeeded++
					case errors.Is(err, service.ErrUserExists):
						duplicate++
					default:
						t.Fatalf("unexpected competing registration error: %v", err)
					}
				}
				if succeeded != 1 || duplicate != 1 {
					t.Fatalf("success=%d duplicate=%d, want one of each", succeeded, duplicate)
				}
			})
		}
	})
}

func TestConcurrentAdminBootstrap(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		start := make(chan struct{})
		results := make(chan error, 2)
		var workers sync.WaitGroup
		for _, name := range []string{"first", "second"} {
			workers.Go(func() {
				<-start
				_, err := fixture.service.InitAdmin(t.Context(), service.RegisterInput{
					Username: name, Email: name + "@example.com", Password: testPassword,
				})
				results <- err
			})
		}
		close(start)
		workers.Wait()
		close(results)
		var succeeded, forbidden int
		for err := range results {
			switch {
			case err == nil:
				succeeded++
			case errors.Is(err, service.ErrForbidden):
				forbidden++
			default:
				t.Fatalf("unexpected competing bootstrap error: %v", err)
			}
		}
		if succeeded != 1 || forbidden != 1 {
			t.Fatalf("success=%d forbidden=%d, want one of each", succeeded, forbidden)
		}
		var count int64
		if err := fixture.db.WithContext(t.Context()).Model(&model.User{}).
			Where("role = ?", "admin").Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("admin count=%d, want one", count)
		}
	})
}
