package http_test

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	httpapi "github.com/biliblihuorong/imgnest/internal/http"
	"github.com/biliblihuorong/imgnest/internal/migrate"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/service"
)

func TestRealDatabaseHTTPAuthLifecycle(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			dsn := filepath.Join(t.TempDir(), "http.db")
			if driver == "postgres" {
				dsn = os.Getenv("IMGNEST_TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("IMGNEST_TEST_POSTGRES_DSN is not set")
				}
			}
			cfg, err := config.Load(t.Context(), "", []string{"IMGNEST_DATABASE_DRIVER=" + driver, "IMGNEST_DATABASE_DSN=" + dsn})
			if err != nil {
				t.Fatal(err)
			}
			if driver == "postgres" {
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
				schema := "http_" + hex.EncodeToString(random[:])
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
						t.Fatal("invalid test DSN")
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
			userRepo, err := repo.NewUserRepository(t.Context(), db)
			if err != nil {
				t.Fatal(err)
			}
			settings, err := repo.NewSettingsRepository(t.Context(), db)
			if err != nil {
				t.Fatal(err)
			}
			tokenRepo, err := repo.NewTokenRepository(t.Context(), db)
			if err != nil {
				t.Fatal(err)
			}
			users, err := service.NewUserService(t.Context(), userRepo, settings)
			if err != nil {
				t.Fatal(err)
			}
			tokens, err := service.NewTokenService(t.Context(), tokenRepo, userRepo, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			const password = "initial-password-123"
			if _, err := users.InitAdmin(t.Context(), service.RegisterInput{Username: "tester", Email: "tester@example.com", Password: password}); err != nil {
				t.Fatal(err)
			}
			router, err := httpapi.NewRouter(t.Context(), httpapi.Dependencies{Users: users, Tokens: tokens, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)), Server: cfg.Server, Now: time.Now, Health: sqlDB.PingContext})
			if err != nil {
				t.Fatal(err)
			}
			login := request(t, router, "POST", "/api/auth/login", `{"email":"tester@example.com","password":"initial-password-123"}`, "")
			expectCode(t, login, 200, 0)
			var session struct {
				Token     string     `json:"token"`
				ExpiresAt *time.Time `json:"expires_at"`
			}
			if err := json.Unmarshal(envelope(t, login)["data"], &session); err != nil {
				t.Fatal(err)
			}
			if session.Token == "" || session.ExpiresAt == nil {
				t.Fatal("login omitted session credential/expiry")
			}
			expectCode(t, request(t, router, "GET", "/api/auth/me", "", session.Token), 200, 0)
			created := request(t, router, "POST", "/api/tokens", `{"name":"integration"}`, session.Token)
			expectCode(t, created, 201, 0)
			var issued service.IssuedToken
			if err := json.Unmarshal(envelope(t, created)["data"], &issued); err != nil {
				t.Fatal(err)
			}
			expectCode(t, request(t, router, "GET", "/api/auth/me", "", issued.Token), 200, 0)
			listed := request(t, router, "GET", "/api/tokens", "", session.Token)
			expectCode(t, listed, 200, 0)
			if strings.Contains(listed.Body.String(), issued.Token) || strings.Contains(listed.Body.String(), "token_hash") {
				t.Fatal("token list exposed credentials")
			}
			expectCode(t, request(t, router, "DELETE", "/api/tokens/"+strings.SplitN(issued.Token, "|", 2)[0], "", session.Token), 200, 0)
			expectCode(t, request(t, router, "GET", "/api/auth/me", "", issued.Token), 401, 20001)
			expectCode(t, request(t, router, "PATCH", "/api/auth/password", `{"current_password":"initial-password-123","new_password":"replacement-password-123"}`, session.Token), 200, 0)
			expectCode(t, request(t, router, "GET", "/api/auth/me", "", session.Token), 401, 20001)
			expectCode(t, request(t, router, "POST", "/api/auth/login", `{"email":"tester@example.com","password":"initial-password-123"}`, ""), 401, 20002)
			fresh := request(t, router, "POST", "/api/auth/login", `{"email":"tester@example.com","password":"replacement-password-123"}`, "")
			expectCode(t, fresh, 200, 0)
			if err := json.Unmarshal(envelope(t, fresh)["data"], &session); err != nil {
				t.Fatal(err)
			}
			expectCode(t, request(t, router, "POST", "/api/auth/logout", "", session.Token), 200, 0)
			expectCode(t, request(t, router, "GET", "/api/auth/me", "", session.Token), 401, 20001)
		})
	}
}
