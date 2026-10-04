package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(t.Context(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Addr != ":8080" {
		t.Errorf("addr = %q, want :8080", cfg.Server.Addr)
	}
	if cfg.Server.ReadHeaderTimeout != 5*time.Second || cfg.Server.ShutdownTimeout != 10*time.Second {
		t.Errorf("unexpected server timeouts: %v, %v", cfg.Server.ReadHeaderTimeout, cfg.Server.ShutdownTimeout)
	}
	if cfg.Server.TrustedProxies == nil || len(cfg.Server.TrustedProxies) != 0 {
		t.Errorf("trusted proxies = %v, want an empty slice", cfg.Server.TrustedProxies)
	}
	want := Database{
		Driver:      "sqlite",
		DSN:         "data/imgnest.db",
		MaxOpen:     1,
		MaxIdle:     1,
		MaxLifetime: 0,
		BusyTimeout: 5 * time.Second,
	}
	if cfg.Database != want {
		t.Errorf("database defaults = %+v, want %+v", cfg.Database, want)
	}
}

func TestLoadPrecedence(t *testing.T) {
	path := writeConfig(t, `server:
  addr: ":8090"
  trusted_proxies: ["127.0.0.1"]
  read_header_timeout: 8s
database:
  driver: postgres
  dsn: host=localhost dbname=imgnest sslmode=disable
  max_open: 30
  max_idle: 5
  max_lifetime: 2m
`)
	cfg, err := Load(t.Context(), path, []string{
		"IMGNEST_SERVER_ADDR=:8091",
		"IMGNEST_SERVER_TRUSTED_PROXIES=127.0.0.1,10.0.0.0/8",
		"IMGNEST_DATABASE_MAX_OPEN=32",
		"IMGNEST_DATABASE_MAX_LIFETIME=3m",
		"IMGNEST_TEST_POSTGRES_DSN=ignored",
		"UNRELATED=value",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Addr != ":8091" {
		t.Errorf("addr = %q, want environment override :8091", cfg.Server.Addr)
	}
	if !slices.Equal(cfg.Server.TrustedProxies, []string{"127.0.0.1", "10.0.0.0/8"}) {
		t.Errorf("trusted proxies = %v, want environment override", cfg.Server.TrustedProxies)
	}
	if cfg.Server.ReadHeaderTimeout != 8*time.Second || cfg.Server.ShutdownTimeout != 10*time.Second {
		t.Error("YAML and default server timeouts were not preserved")
	}
	if cfg.Database.MaxOpen != 32 || cfg.Database.MaxIdle != 5 || cfg.Database.MaxLifetime != 3*time.Minute {
		t.Error("database fields with internal underscores were not overridden correctly")
	}
	if cfg.Database.DSN != "host=localhost dbname=imgnest sslmode=disable" {
		t.Error("YAML DSN was not preserved")
	}
}

func TestLoadDriverDefaults(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		environ []string
		driver  string
		open    int
		idle    int
		life    time.Duration
	}{
		{
			name:    "postgres from YAML",
			yaml:    "database:\n  driver: postgres\n  dsn: host=localhost dbname=imgnest\n",
			environ: []string{},
			driver:  "postgres",
			open:    25,
			idle:    10,
			life:    5 * time.Minute,
		},
		{
			name:    "postgres from environment",
			yaml:    "",
			environ: []string{"IMGNEST_DATABASE_DRIVER=postgres", "IMGNEST_DATABASE_DSN=host=localhost dbname=imgnest"},
			driver:  "postgres",
			open:    25,
			idle:    10,
			life:    5 * time.Minute,
		},
		{
			name:    "environment selects SQLite defaults",
			yaml:    "database:\n  driver: postgres\n  dsn: data/override.db\n",
			environ: []string{"IMGNEST_DATABASE_DRIVER=sqlite"},
			driver:  "sqlite",
			open:    1,
			idle:    1,
			life:    0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := ""
			if tt.yaml != "" {
				path = writeConfig(t, tt.yaml)
			}
			cfg, err := Load(t.Context(), path, tt.environ)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Database.Driver != tt.driver || cfg.Database.MaxOpen != tt.open || cfg.Database.MaxIdle != tt.idle {
				t.Error("pool defaults do not match the selected database driver")
			}
			if cfg.Database.MaxLifetime != tt.life {
				t.Errorf("max lifetime = %v, want %v", cfg.Database.MaxLifetime, tt.life)
			}
		})
	}
}

func TestLoadOnlyUsesProvidedEnvironment(t *testing.T) {
	t.Setenv("IMGNEST_SERVER_ADDR", ":9876")
	cfg, err := Load(t.Context(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Addr != ":8080" {
		t.Error("Load read the process environment instead of the provided environment")
	}
}

func TestLoadEnvironmentFields(t *testing.T) {
	path := writeConfig(t, "server:\n  trusted_proxies: [127.0.0.1]\n")
	cfg, err := Load(t.Context(), path, []string{
		"IMGNEST_SERVER_TRUSTED_PROXIES=",
		"IMGNEST_SERVER_READ_HEADER_TIMEOUT=2s",
		"IMGNEST_SERVER_SHUTDOWN_TIMEOUT=3s",
		"IMGNEST_DATABASE_DSN=file:override.db?cache=shared&mode=rwc",
		"IMGNEST_DATABASE_MAX_OPEN=2",
		"IMGNEST_DATABASE_MAX_IDLE=0",
		"IMGNEST_DATABASE_MAX_LIFETIME=1m",
		"IMGNEST_DATABASE_BUSY_TIMEOUT=1234ms",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Server.TrustedProxies) != 0 || cfg.Server.TrustedProxies == nil {
		t.Error("an empty environment value did not clear trusted proxies")
	}
	if cfg.Server.ReadHeaderTimeout != 2*time.Second || cfg.Server.ShutdownTimeout != 3*time.Second {
		t.Error("server timeout environment overrides were not applied")
	}
	if cfg.Database.DSN != "file:override.db?cache=shared&mode=rwc" {
		t.Error("DSN environment override containing equals signs was not preserved")
	}
	if cfg.Database.MaxOpen != 2 || cfg.Database.MaxIdle != 0 {
		t.Error("pool environment overrides were not applied")
	}
	if cfg.Database.MaxLifetime != time.Minute || cfg.Database.BusyTimeout != 1234*time.Millisecond {
		t.Error("database duration environment overrides were not applied")
	}
}

func TestLoadMalformedEnvironment(t *testing.T) {
	_, err := Load(t.Context(), "", []string{"IMGNEST_DATABASE_DSN"})
	if err == nil {
		t.Fatal("environment assignment without equals sign was accepted")
	}
}

func TestLoadInvalidDriver(t *testing.T) {
	_, err := Load(t.Context(), "", []string{"IMGNEST_DATABASE_DRIVER=mysql"})
	if err == nil {
		t.Fatal("unsupported driver was accepted")
	}
}

func TestLoadPostgresRequiresDSN(t *testing.T) {
	_, err := Load(t.Context(), "", []string{"IMGNEST_DATABASE_DRIVER=postgres"})
	if err == nil {
		t.Fatal("PostgreSQL without an explicit DSN was accepted")
	}
}

func TestLoadMissingExplicitFile(t *testing.T) {
	_, err := Load(t.Context(), filepath.Join(t.TempDir(), "missing.yaml"), nil)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, want os.ErrNotExist", err)
	}
}

func TestLoadCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := Load(ctx, "", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestLoadInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "IMGNEST_SERVER_ADDR", value: ""},
		{name: "IMGNEST_SERVER_ADDR", value: "localhost"},
		{name: "IMGNEST_SERVER_TRUSTED_PROXIES", value: "trusted.example.com"},
		{name: "IMGNEST_SERVER_READ_HEADER_TIMEOUT", value: "garbage"},
		{name: "IMGNEST_SERVER_READ_HEADER_TIMEOUT", value: "-1s"},
		{name: "IMGNEST_SERVER_SHUTDOWN_TIMEOUT", value: "-1s"},
		{name: "IMGNEST_DATABASE_DSN", value: ""},
		{name: "IMGNEST_DATABASE_MAX_OPEN", value: "zero"},
		{name: "IMGNEST_DATABASE_MAX_OPEN", value: "1.5"},
		{name: "IMGNEST_DATABASE_MAX_OPEN", value: "0"},
		{name: "IMGNEST_DATABASE_MAX_IDLE", value: "-1"},
		{name: "IMGNEST_DATABASE_MAX_IDLE", value: "2"},
		{name: "IMGNEST_DATABASE_MAX_LIFETIME", value: "-1s"},
		{name: "IMGNEST_DATABASE_BUSY_TIMEOUT", value: "-1ms"},
	}
	for _, tt := range tests {
		t.Run(tt.name+"="+tt.value, func(t *testing.T) {
			_, err := Load(t.Context(), "", []string{tt.name + "=" + tt.value})
			if err == nil {
				t.Fatal("invalid configuration was accepted")
			}
		})
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	for _, content := range []string{
		"server: [\n",
		"server:\n  read_header_timeout: [5s]\n",
		"database:\n  max_open: 1.5\n",
		"database:\n  max_open: null\n",
		"server:\n  trusted_proxies: 127.0.0.1\n",
	} {
		t.Run(content, func(t *testing.T) {
			_, err := Load(t.Context(), writeConfig(t, content), nil)
			if err == nil {
				t.Fatal("malformed or ill-typed YAML was accepted")
			}
		})
	}
}

func TestConfigErrorsRedactSecrets(t *testing.T) {
	const secret = "test-only-dsn-password" // #nosec G101 -- invented redaction test fixture, not a real credential.
	for _, content := range []string{
		"database:\n  dsn: postgres://user:" + secret + "@localhost/imgnest\n  max_open: invalid\n",
		"database:\n  dsn: [postgres://user:" + secret + "@localhost/imgnest]\n",
		"database:\n  dsn: postgres://user:" + secret + "@localhost/imgnest\n  max_lifetime: " + secret + "\n",
		"database:\n  dsn: [postgres://user:" + secret + "@localhost/imgnest\n",
	} {
		_, err := Load(t.Context(), writeConfig(t, content), nil)
		if err == nil {
			t.Fatal("invalid configuration was accepted")
		}
		for cause := err; cause != nil; cause = errors.Unwrap(cause) {
			if strings.Contains(cause.Error(), secret) {
				t.Fatal("configuration error exposes a DSN password")
			}
		}
	}
	_, err := Load(t.Context(), "", []string{"IMGNEST_DATABASE_DRIVER=" + secret})
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatal("invalid environment value was accepted or exposed in an error")
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
