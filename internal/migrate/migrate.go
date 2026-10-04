// Package migrate applies explicit, versioned database migrations.
package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed sqlite/*.sql postgres/*.sql
var files embed.FS

type migration struct {
	version int64
	sql     string
}

func (m migration) checksum() string {
	digest := sha256.Sum256([]byte(m.sql))
	return hex.EncodeToString(digest[:])
}

// Up applies all pending migrations.
func Up(ctx context.Context, db *sql.DB, driver string) error {
	scripts, err := manifest(driver)
	if err != nil {
		return err
	}
	return up(ctx, db, driver, scripts)
}

// Check verifies that the database matches the binary's migration manifest.
func Check(ctx context.Context, db *sql.DB, driver string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("check migrations: %w", err)
	}
	if db == nil {
		return errors.New("database is required")
	}
	scripts, err := manifest(driver)
	if err != nil {
		return err
	}
	for _, table := range []string{"schema_migrations", "groups", "users", "tokens", "settings"} {
		exists, err := hasTable(ctx, db, driver, table)
		if err != nil {
			return migrationError("inspect schema", err)
		}
		if !exists {
			return errors.New("database schema is missing; run migrate")
		}
	}
	applied, err := readApplied(ctx, db)
	if err != nil {
		return migrationError("read migration versions", err)
	}
	if err := verifyApplied(scripts, applied); err != nil {
		return err
	}
	if len(applied) != len(scripts) {
		return errors.New("database has pending migrations; run migrate")
	}
	return nil
}

func up(ctx context.Context, db *sql.DB, driver string, scripts []migration) (result error) {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	if db == nil {
		return errors.New("database is required")
	}
	if driver != "sqlite" && driver != "postgres" {
		return errors.New("unsupported migration driver")
	}
	if len(scripts) == 0 {
		return errors.New("empty migration manifest")
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return migrationError("acquire migration connection", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			result = errors.Join(result, migrationError("close migration connection", err))
		}
	}()
	for _, script := range scripts {
		if err := apply(ctx, conn, driver, scripts, script); err != nil {
			return fmt.Errorf("migration %04d: %w", script.version, err)
		}
	}
	return nil
}

func apply(
	ctx context.Context,
	conn *sql.Conn,
	driver string,
	scripts []migration,
	script migration,
) (result error) {
	begin := "BEGIN"
	if driver == "sqlite" {
		begin = "BEGIN IMMEDIATE"
	}
	if _, err := conn.ExecContext(ctx, begin); err != nil {
		return migrationError("begin migration", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(cleanup, "ROLLBACK"); err != nil {
			result = errors.Join(result, migrationError("rollback migration", err))
		}
	}()
	if driver == "postgres" {
		// Transaction-scoped and schema-scoped locks avoid leaking a session lock into the pool.
		if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext(current_schema()), 1229801281)"); err != nil {
			return migrationError("lock migration", err)
		}
	}
	metadata := `CREATE TABLE IF NOT EXISTS schema_migrations (
		version BIGINT PRIMARY KEY, checksum TEXT NOT NULL, applied_at DATETIME NOT NULL)`
	if driver == "postgres" {
		metadata = `CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY, checksum TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL)`
	}
	if _, err := conn.ExecContext(ctx, metadata); err != nil {
		return migrationError("create migration metadata", err)
	}
	applied, err := readApplied(ctx, conn)
	if err != nil {
		return migrationError("read migration versions", err)
	}
	if err := verifyApplied(scripts, applied); err != nil {
		return err
	}
	if _, exists := applied[script.version]; !exists {
		if _, err := conn.ExecContext(ctx, script.sql); err != nil {
			return migrationError("execute migration SQL", err)
		}
		insert := "INSERT INTO schema_migrations (version, checksum, applied_at) VALUES (?, ?, ?)"
		if driver == "postgres" {
			insert = "INSERT INTO schema_migrations (version, checksum, applied_at) VALUES ($1, $2, $3)"
		}
		if _, err := conn.ExecContext(
			ctx,
			insert,
			script.version,
			script.checksum(),
			time.Now().UTC(),
		); err != nil {
			return migrationError("record migration version", err)
		}
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return migrationError("commit migration", err)
	}
	committed = true
	return nil
}

func manifest(driver string) ([]migration, error) {
	if driver != "sqlite" && driver != "postgres" {
		return nil, errors.New("unsupported migration driver")
	}
	paths, err := fs.Glob(files, driver+"/*.sql")
	if err != nil {
		return nil, fmt.Errorf("read migration manifest: %w", err)
	}
	scripts := make([]migration, 0, len(paths))
	for _, path := range paths {
		name, _, _ := strings.Cut(strings.TrimPrefix(path, driver+"/"), "_")
		version, err := strconv.ParseInt(name, 10, 64)
		if err != nil || version <= 0 {
			return nil, errors.New("invalid migration version")
		}
		body, err := files.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read migration SQL: %w", err)
		}
		scripts = append(scripts, migration{version: version, sql: string(body)})
	}
	sort.Slice(scripts, func(i, j int) bool { return scripts[i].version < scripts[j].version })
	return scripts, nil
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readApplied(ctx context.Context, db queryer) (applied map[int64]string, result error) {
	rows, err := db.QueryContext(ctx, "SELECT version, checksum FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			result = errors.Join(result, err)
		}
	}()
	applied = make(map[int64]string)
	for rows.Next() {
		var version int64
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return nil, err
		}
		applied[version] = checksum
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return applied, nil
}

func verifyApplied(scripts []migration, applied map[int64]string) error {
	known := make(map[int64]string, len(scripts))
	for _, script := range scripts {
		known[script.version] = script.checksum()
	}
	for version, checksum := range applied {
		expected, exists := known[version]
		if !exists {
			return errors.New("database contains an unknown migration version")
		}
		if checksum != expected {
			return errors.New("database migration checksum mismatch")
		}
	}
	pending := false
	for _, script := range scripts {
		if _, exists := applied[script.version]; !exists {
			pending = true
			continue
		}
		if pending {
			return errors.New("database migration history has a missing version")
		}
	}
	return nil
}

func hasTable(ctx context.Context, db queryer, driver, name string) (bool, error) {
	query := "SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?)"
	if driver == "postgres" {
		query = `SELECT EXISTS (SELECT 1 FROM information_schema.tables
			WHERE table_schema = current_schema() AND table_name = $1)`
	}
	var exists bool
	err := db.QueryRowContext(ctx, query, name).Scan(&exists)
	return exists, err
}

type redactedMigrationError struct{ cause error }

func (e redactedMigrationError) Error() string { return "database operation failed" }
func (e redactedMigrationError) Unwrap() error { return e.cause }

func migrationError(operation string, err error) error {
	return fmt.Errorf("%s: %w", operation, redactedMigrationError{cause: err})
}
