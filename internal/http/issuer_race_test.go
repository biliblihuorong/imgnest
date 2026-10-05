package http_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	httpapi "github.com/biliblihuorong/imgnest/internal/http"
	"github.com/biliblihuorong/imgnest/internal/migrate"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/service"
)

type raceFixture struct {
	users  *service.UserService
	tokens *service.TokenService
	admin  service.UserView
	cfg    config.Config
	health func(context.Context) error
}

const racePassword = "credential-race-password" // #nosec G101 -- invented isolated-test credential, never a deployment secret.

func newRaceFixture(t *testing.T, driver string) *raceFixture {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "race.db")
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
		sqlDB, err := admin.DB()
		if err != nil {
			t.Fatal(err)
		}
		var name [8]byte
		if _, err := rand.Read(name[:]); err != nil {
			t.Fatal(err)
		}
		schema := "grant_" + hex.EncodeToString(name[:])
		if _, err := sqlDB.ExecContext(t.Context(), "CREATE SCHEMA "+schema); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := sqlDB.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
				t.Error(err)
			}
			if err := sqlDB.Close(); err != nil {
				t.Error(err)
			}
		})
		if strings.Contains(dsn, "://") {
			parsed, err := url.Parse(dsn)
			if err != nil {
				t.Fatal("invalid test DSN")
			}
			q := parsed.Query()
			q.Set("search_path", schema)
			parsed.RawQuery = q.Encode()
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
	usersRepo, err := repo.NewUserRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := repo.NewSettingsRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	tokensRepo, err := repo.NewTokenRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	users, err := service.NewUserService(t.Context(), usersRepo, settings)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := service.NewTokenService(t.Context(), tokensRepo, usersRepo, settings, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := users.InitAdmin(t.Context(), service.RegisterInput{Username: "race-user", Email: "race@example.com", Password: racePassword})
	if err != nil {
		t.Fatal(err)
	}
	return &raceFixture{users: users, tokens: tokens, admin: admin, cfg: cfg, health: sqlDB.PingContext}
}
func raceRouter(t *testing.T, f *raceFixture, users httpapiUserService) http.Handler {
	t.Helper()
	router, err := httpapi.NewRouter(t.Context(), httpapi.Dependencies{Users: users, Tokens: f.tokens, Server: f.cfg.Server, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)), Now: time.Now, Health: f.health})
	if err != nil {
		t.Fatal(err)
	}
	return router
}

type verifiedCredential = service.VerifiedCredentials
type httpapiUserService interface {
	Register(context.Context, service.RegisterInput) (service.UserView, error)
	VerifyCredentials(context.Context, string, string) (verifiedCredential, error)
	ChangePassword(context.Context, uint64, string, string) error
	UpdateDisplayName(context.Context, uint64, string) (service.UserView, error)
	Site(context.Context) (service.SiteView, error)
}

type heldBody struct {
	reader  io.Reader
	ready   chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *heldBody) Read(p []byte) (int, error) {
	b.once.Do(func() { close(b.ready) })
	<-b.release
	return b.reader.Read(p)
}

func TestRevokedRequestCannotIssueToken(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		for _, action := range []string{"reset", "revoke"} {
			t.Run(driver+"/"+action, func(t *testing.T) {
				f := newRaceFixture(t, driver)
				router := raceRouter(t, f, f.users)
				login := request(t, router, "POST", "/api/auth/login", `{"email":"race@example.com","password":"credential-race-password"}`, "")
				if login.Code != 200 {
					t.Fatal("fixture login failed")
				}
				var session struct {
					Token string `json:"token"`
				}
				if err := json.Unmarshal(envelope(t, login)["data"], &session); err != nil {
					t.Fatal(err)
				}
				body := &heldBody{reader: strings.NewReader(`{"name":"delayed-issue"}`), ready: make(chan struct{}), release: make(chan struct{})}
				r := httptest.NewRequest("POST", "/api/tokens", body)
				r.RemoteAddr = "192.0.2.1:1234"
				r.Header.Set("Authorization", "Bearer "+session.Token)
				response := httptest.NewRecorder()
				finished := make(chan struct{})
				go func() { router.ServeHTTP(response, r); close(finished) }()
				select {
				case <-body.ready:
				case <-time.After(3 * time.Second):
					close(body.release)
					t.Fatal("request did not finish initial authentication")
				}
				if action == "reset" {
					if err := f.users.ResetPassword(t.Context(), f.admin.Email, "reset-race-password"); err != nil {
						close(body.release)
						t.Fatal(err)
					}
				} else {
					tokenID, err := strconv.ParseUint(strings.SplitN(session.Token, "|", 2)[0], 10, 64)
					if err != nil {
						close(body.release)
						t.Fatal(err)
					}
					if err := f.tokens.Revoke(t.Context(), f.admin.ID, tokenID); err != nil {
						close(body.release)
						t.Fatal(err)
					}
				}
				close(body.release)
				select {
				case <-finished:
				case <-time.After(3 * time.Second):
					t.Fatal("request did not resume")
				}
				if response.Code != 401 {
					t.Fatalf("revoked request issued token: HTTP=%d, want401", response.Code)
				}
				var payload struct {
					Code int `json:"code"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				if payload.Code != 20001 {
					t.Fatalf("revoked request code=%d, want20001", payload.Code)
				}
			})
		}
	}
}

type heldCredentials struct {
	*service.UserService
	ready   chan struct{}
	release chan struct{}
}

func (s *heldCredentials) VerifyCredentials(ctx context.Context, email, password string) (verifiedCredential, error) {
	value, err := s.UserService.VerifyCredentials(ctx, email, password)
	if err == nil {
		close(s.ready)
		<-s.release
	}
	return value, err
}
func TestResetInvalidatesInFlightLogin(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			f := newRaceFixture(t, driver)
			paused := &heldCredentials{UserService: f.users, ready: make(chan struct{}), release: make(chan struct{})}
			router := raceRouter(t, f, paused)
			r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"email":"race@example.com","password":"credential-race-password"}`))
			r.RemoteAddr = "192.0.2.1:1234"
			response := httptest.NewRecorder()
			finished := make(chan struct{})
			go func() { router.ServeHTTP(response, r); close(finished) }()
			select {
			case <-paused.ready:
			case <-finished:
				close(paused.release)
				t.Fatalf("login finished before password verification barrier: HTTP=%d", response.Code)
			case <-time.After(15 * time.Second):
				// Real cost-12 bcrypt can exceed three seconds under race
				// instrumentation on constrained executors. Keep a bounded
				// wait without weakening the reset/token ordering assertion.
				close(paused.release)
				t.Fatal("password verification barrier not reached")
			}
			if err := f.users.ResetPassword(t.Context(), f.admin.Email, "reset-race-password"); err != nil {
				close(paused.release)
				t.Fatal(err)
			}
			close(paused.release)
			select {
			case <-finished:
			case <-time.After(3 * time.Second):
				t.Fatal("login did not resume")
			}
			if response.Code != 401 {
				t.Fatalf("stale login issued token: HTTP=%d, want401", response.Code)
			}
		})
	}
}
