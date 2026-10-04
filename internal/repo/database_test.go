package repo

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
)

func TestSQLitePragmas(t *testing.T) {
	db, err := Open(t.Context(), config.Database{
		Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "nested", "test.db"),
		MaxOpen: 8, MaxIdle: 4, BusyTimeout: 2500 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := sqlDB.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	var journal string
	if err := sqlDB.QueryRowContext(t.Context(), "PRAGMA journal_mode").Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if journal != "wal" {
		t.Fatalf("journal_mode=%q, want wal", journal)
	}
	for name, want := range map[string]int{"foreign_keys": 1, "busy_timeout": 2500} {
		var got int
		if err := sqlDB.QueryRowContext(t.Context(), "PRAGMA "+name).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s=%d, want %d", name, got, want)
		}
	}
	if got := sqlDB.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("max_open=%d, want 1", got)
	}
}

func TestOpenCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := Open(ctx, config.Database{Driver: "sqlite", DSN: ":memory:"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want context.Canceled", err)
	}
}

func TestOpenErrorsRedactCredentials(t *testing.T) {
	const secret = "database-credential-must-not-leak"
	_, err := Open(t.Context(), config.Database{
		Driver: "postgres", DSN: "postgres://user:" + secret + "@invalid.invalid/db?sslmode=invalid",
		MaxOpen: 1, MaxIdle: 1,
	})
	if err == nil {
		t.Fatal("invalid DSN accepted")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error contains credential: %s", err.Error())
	}
}
