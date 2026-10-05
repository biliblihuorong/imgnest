package migrate

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/repo"
)

func TestMigrateFreshDatabase(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *sql.DB, driver string) {
		if err := Up(t.Context(), db, driver); err != nil {
			t.Fatal(err)
		}
		for _, table := range []string{
			"groups", "users", "tokens", "settings", "schema_migrations",
			"storages", "policies", "group_policies", "albums", "images", "image_exif",
		} {
			var count int
			if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
				t.Fatalf("table %s unavailable: %v", table, err)
			}
		}
		if err := Check(t.Context(), db, driver); err != nil {
			t.Fatal(err)
		}
	})
}

func TestMigrateTwice(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *sql.DB, driver string) {
		for range 2 {
			if err := Up(t.Context(), db, driver); err != nil {
				t.Fatal(err)
			}
		}
		var defaults, settings int
		if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM groups WHERE is_default = true").Scan(&defaults); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM settings").Scan(&settings); err != nil {
			t.Fatal(err)
		}
		if defaults != 1 || settings != 8 {
			t.Fatalf("default groups=%d, settings=%d; want 1 and 8", defaults, settings)
		}
		var registration string
		if err := db.QueryRowContext(t.Context(), "SELECT value FROM settings WHERE key = 'registration_enabled'").Scan(&registration); err != nil {
			t.Fatal(err)
		}
		if registration != "false" {
			t.Fatalf("registration=%s, want false", registration)
		}
	})
}

func TestMigrationFailureRollsBack(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *sql.DB, driver string) {
		scripts := []migration{{version: 1, sql: "CREATE TABLE rolled_back (id BIGINT); INVALID SQL;"}}
		if err := up(t.Context(), db, driver, scripts); err == nil {
			t.Fatal("invalid migration succeeded")
		}
		var exists bool
		if err := testTableExists(t.Context(), db, driver, "rolled_back").Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Fatal("failed migration left its table behind")
		}
		if err := testTableExists(t.Context(), db, driver, "schema_migrations").Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Fatal("failed initial migration left version metadata behind")
		}
	})
}

func TestMigrationFailureKeepsPreviouslyCommittedVersions(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *sql.DB, driver string) {
		scripts := []migration{
			{version: 1, sql: "CREATE TABLE committed (id BIGINT);"},
			{version: 2, sql: "CREATE TABLE rolled_back (id BIGINT); INVALID SQL;"},
		}
		if err := up(t.Context(), db, driver, scripts); err == nil {
			t.Fatal("invalid migration succeeded")
		}
		var committed, rolledBack bool
		if err := testTableExists(t.Context(), db, driver, "committed").Scan(&committed); err != nil {
			t.Fatal(err)
		}
		if err := testTableExists(t.Context(), db, driver, "rolled_back").Scan(&rolledBack); err != nil {
			t.Fatal(err)
		}
		if !committed || rolledBack {
			t.Fatalf("committed table=%v rolled-back table=%v, want true/false", committed, rolledBack)
		}
		var versions int
		if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM schema_migrations WHERE version = 1").Scan(&versions); err != nil {
			t.Fatal(err)
		}
		if versions != 1 {
			t.Fatalf("previous version records=%d, want 1", versions)
		}
	})
}

func TestConcurrentMigrations(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *sql.DB, driver string) {
		start := make(chan struct{})
		results := make(chan error, 2)
		for range 2 {
			go func() {
				<-start
				results <- Up(t.Context(), db, driver)
			}()
		}
		close(start)
		for range 2 {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
		var count int
		if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
			t.Fatal(err)
		}
		scripts, err := manifest(driver)
		if err != nil {
			t.Fatal(err)
		}
		if count != len(scripts) {
			t.Fatalf("concurrent migrations recorded %d versions, want %d", count, len(scripts))
		}
	})
}

func TestCheckRejectsMissingBusinessTable(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *sql.DB, driver string) {
		if err := Up(t.Context(), db, driver); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), "DROP TABLE tokens"); err != nil {
			t.Fatal(err)
		}
		if err := Check(t.Context(), db, driver); err == nil {
			t.Fatal("check accepted missing business table")
		}
	})
}

func testTableExists(ctx context.Context, db *sql.DB, driver, name string) *sql.Row {
	if driver == "postgres" {
		return db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1)", name)
	}
	return db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?)", name)
}

func TestMigrationChecksumMismatch(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *sql.DB, driver string) {
		if err := Up(t.Context(), db, driver); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), "UPDATE schema_migrations SET checksum = 'modified'"); err != nil {
			t.Fatal(err)
		}
		if err := Check(t.Context(), db, driver); err == nil {
			t.Fatal("check accepted modified migration")
		}
		if err := Up(t.Context(), db, driver); err == nil {
			t.Fatal("up accepted modified migration")
		}
	})
}

func TestCheckPendingAndUnknownVersion(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *sql.DB, driver string) {
		if err := Check(t.Context(), db, driver); err == nil {
			t.Fatal("check accepted empty database")
		}
		if err := Up(t.Context(), db, driver); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), "UPDATE schema_migrations SET version = 99 WHERE version = 1"); err != nil {
			t.Fatal(err)
		}
		if err := Check(t.Context(), db, driver); err == nil {
			t.Fatal("check accepted future migration")
		}
		if err := Up(t.Context(), db, driver); err == nil {
			t.Fatal("up accepted future migration")
		}
	})
}

func TestMigrateM1ToImageCore(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *sql.DB, driver string) {
		scripts, err := manifest(driver)
		if err != nil {
			t.Fatal(err)
		}
		if err := up(t.Context(), db, driver, scripts[:1]); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), `INSERT INTO users
			(username,email,password_hash,role,group_id,status,used_bytes)
			VALUES ('existing','existing@example.com','digest','user',1,'enabled',17)`); err != nil {
			t.Fatal(err)
		}
		if err := Up(t.Context(), db, driver); err != nil {
			t.Fatal(err)
		}
		var used int64
		if err := db.QueryRowContext(t.Context(), "SELECT used_bytes FROM users WHERE username='existing'").Scan(&used); err != nil {
			t.Fatal(err)
		}
		if used != 17 {
			t.Fatalf("M1 user changed during upgrade: used=%d", used)
		}
		var table bool
		if err := testTableExists(t.Context(), db, driver, "images").Scan(&table); err != nil {
			t.Fatal(err)
		}
		if !table {
			t.Fatal("M1 upgrade did not create images")
		}
		if err := Check(t.Context(), db, driver); err != nil {
			t.Fatal(err)
		}
	})
}

func TestM1MigrationChecksumsRemainUnchanged(t *testing.T) {
	for driver, want := range map[string]string{
		"sqlite":   "90ef153625f486a1b7a808373a0f09523f24aa9083c0d67f3c400e8c21fc3248",
		"postgres": "e78d55e4dc05286d199428f59fbd9d42a806bdcb5fe212c3123da8aa70d24b19",
	} {
		t.Run(driver, func(t *testing.T) {
			scripts, err := manifest(driver)
			if err != nil {
				t.Fatal(err)
			}
			if got := scripts[0].checksum(); got != want {
				t.Fatalf("published 0001 changed: checksum=%s", got)
			}
		})
	}
}

func TestMigrationCancelledContext(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *sql.DB, driver string) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := Up(ctx, db, driver); !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v, want context.Canceled", err)
		}
	})
}

func forEachDatabase(t *testing.T, run func(*testing.T, *sql.DB, string)) {
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
				cfg.DSN = isolatePostgres(t, cfg)
			}
			gormDB, err := repo.Open(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			db, err := gormDB.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := db.Close(); err != nil {
					t.Error(err)
				}
			})
			run(t, db, driver)
		})
	}
}

func isolatePostgres(t *testing.T, cfg config.Database) string {
	t.Helper()
	admin, err := repo.Open(t.Context(), cfg)
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
	schema := "imgnest_test_" + hex.EncodeToString(random[:])
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
