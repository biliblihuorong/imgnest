// Package repo implements ImgNest persistence.
package repo

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open creates a database connection using validated deployment configuration.
func Open(ctx context.Context, cfg config.Database) (*gorm.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	var dialector gorm.Dialector
	switch cfg.Driver {
	case "sqlite":
		dsn, err := sqliteDSN(cfg)
		if err != nil {
			return nil, databaseError("configure sqlite", err)
		}
		dialector = sqlite.Open(dsn)
	case "postgres":
		dialector = postgres.Open(cfg.DSN)
	default:
		return nil, errors.New("unsupported database driver")
	}
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger:               logger.Default.LogMode(logger.Silent),
		DisableAutomaticPing: true,
		TranslateError:       true,
		NowFunc:              func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return nil, databaseError("open database", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, databaseError("access connection pool", err)
	}
	if cfg.Driver == "sqlite" {
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
		sqlDB.SetConnMaxLifetime(0)
	} else {
		sqlDB.SetMaxOpenConns(cfg.MaxOpen)
		sqlDB.SetMaxIdleConns(cfg.MaxIdle)
		sqlDB.SetConnMaxLifetime(cfg.MaxLifetime)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		if closeErr := sqlDB.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
		return nil, databaseError("ping database", err)
	}
	return db, nil
}

func sqliteDSN(cfg config.Database) (string, error) {
	base, rawQuery, _ := strings.Cut(cfg.DSN, "?")
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", err
	}
	file := strings.TrimPrefix(base, "file:")
	inMemory := file == ":memory:" || query.Get("mode") == "memory"
	if !inMemory {
		file, err = url.PathUnescape(file)
		if err != nil {
			return "", err
		}
		if file == "" {
			return "", errors.New("empty sqlite filename")
		}
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			return "", err
		}
		query.Del("_journal")
		query.Set("_journal_mode", "WAL")
	}
	query.Del("_fk")
	query.Set("_foreign_keys", "on")
	query.Del("_timeout")
	busyTimeout := cfg.BusyTimeout
	if busyTimeout == 0 {
		busyTimeout = 5 * time.Second
	}
	query.Set("_busy_timeout", strconv.FormatInt(busyTimeout.Milliseconds(), 10))
	query.Set("_txlock", "immediate")
	return base + "?" + query.Encode(), nil
}

// Error strings omit driver diagnostics, which can contain DSNs or SQL values.
// Unwrap preserves cancellation and driver classification without exposing them in logs.
type redactedDatabaseError struct{ cause error }

func (e redactedDatabaseError) Error() string { return "database operation failed" }
func (e redactedDatabaseError) Unwrap() error { return e.cause }

func databaseError(operation string, err error) error {
	return fmt.Errorf("%s: %w", operation, redactedDatabaseError{cause: err})
}

func checkRecordID(ctx context.Context, id uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Both supported databases generate IDs within signed BIGINT range.
	if id > math.MaxInt64 {
		return model.ErrNotFound
	}
	return nil
}
