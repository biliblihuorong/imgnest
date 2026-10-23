package repo

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// backupTables lists every data table in foreign-key order: each table only
// references tables before it, except albums.cover_image_id, which points
// forward at images and is restored by a trailing UPDATE. schema_migrations
// is not data; the restore target runs the same migrations instead. A test
// fails when a migration adds a table this list does not cover.
var backupTables = []string{
	"settings", "storages", "policies", "groups", "group_policies", "users", "tokens",
	"login_events", "api_usage_daily",
	"user_identities", "albums", "images", "image_exif", "image_variant_jobs",
	"variant_backfill", "random_links", "email_codes", "plugin_counters",
	"plugin_user_settings",
	"billing_wallets", "billing_plans", "billing_orders", "billing_entries", "billing_memberships", "billing_requests",
}

// backupDeferred names the forward reference written as NULL first and
// filled in once its target rows exist.
var backupDeferred = map[string]string{"albums": "cover_image_id"}

// BackupRepository dumps and restores the whole database as SQL INSERT
// statements in the dialect of the database it reads.
type BackupRepository struct{ db *gorm.DB }

// NewBackupRepository constructs the backup adapter.
func NewBackupRepository(ctx context.Context, db *gorm.DB) (*BackupRepository, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("create backup repository: %w", err)
	}
	if db == nil {
		return nil, fmt.Errorf("create backup repository: %w", model.ErrInvalidInput)
	}
	return &BackupRepository{db: db}, nil
}

// Dump writes a consistent snapshot of every data table to w and reports the
// driver, schema version and row count per table.
func (r *BackupRepository) Dump(ctx context.Context, w io.Writer) (model.BackupStats, error) {
	stats := model.BackupStats{Driver: r.db.Name(), Tables: map[string]int64{}}
	sqlDB, err := r.db.DB()
	if err != nil {
		return stats, databaseError("access connection pool", err)
	}
	opts := &sql.TxOptions{ReadOnly: true}
	if stats.Driver == "postgres" {
		opts.Isolation = sql.LevelRepeatableRead
	}
	tx, err := sqlDB.BeginTx(ctx, opts)
	if err != nil {
		return stats, databaseError("begin backup snapshot", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&stats.SchemaVersion); err != nil {
		return stats, databaseError("read schema version", err)
	}
	out := bufio.NewWriter(w)
	if _, err := fmt.Fprintf(out, "-- ImgNest backup: %s schema %d\n", stats.Driver, stats.SchemaVersion); err != nil {
		return stats, fmt.Errorf("write backup: %w", err)
	}
	var deferred []string
	for _, table := range backupTables {
		count, updates, err := dumpTable(ctx, tx, out, stats.Driver, table)
		if err != nil {
			return stats, err
		}
		stats.Tables[table] = count
		deferred = append(deferred, updates...)
	}
	for _, statement := range deferred {
		if _, err := out.WriteString(statement); err != nil {
			return stats, fmt.Errorf("write backup: %w", err)
		}
	}
	if err := out.Flush(); err != nil {
		return stats, fmt.Errorf("write backup: %w", err)
	}
	return stats, nil
}

func dumpTable(ctx context.Context, tx *sql.Tx, out *bufio.Writer, driver, table string) (int64, []string, error) {
	rows, err := tx.QueryContext(ctx, "SELECT * FROM "+quoteIdent(table)) //nolint:gosec // table names come from backupTables
	if err != nil {
		return 0, nil, databaseError("read "+table, err)
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		return 0, nil, databaseError("read "+table+" columns", err)
	}
	quoted := make([]string, len(columns))
	deferredAt, idAt := -1, -1
	for i, column := range columns {
		quoted[i] = quoteIdent(column)
		if column == backupDeferred[table] {
			deferredAt = i
		}
		if column == "id" {
			idAt = i
		}
	}
	prefix := "INSERT INTO " + quoteIdent(table) + " (" + strings.Join(quoted, ", ") + ") VALUES ("
	values := make([]any, len(columns))
	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}
	var count int64
	var updates []string
	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return 0, nil, databaseError("read "+table+" row", err)
		}
		literals := make([]string, len(values))
		for i, value := range values {
			if table == "plugin_user_settings" && columns[i] == "asset" && value != nil {
				data, ok := value.([]byte)
				if !ok {
					return 0, nil, model.ErrInvalidInput
				}
				encoded := hex.EncodeToString(data)
				literals[i] = "X'" + encoded + "'"
				if driver == "postgres" {
					literals[i] = "'\\x" + encoded + "'::bytea"
				}
				continue
			}
			literal, err := sqlLiteral(driver, value)
			if err != nil {
				return 0, nil, fmt.Errorf("encode %s.%s: %w", table, columns[i], err)
			}
			literals[i] = literal
		}
		if deferredAt >= 0 && literals[deferredAt] != "NULL" && idAt >= 0 {
			updates = append(updates, "UPDATE "+quoteIdent(table)+" SET "+quoted[deferredAt]+" = "+literals[deferredAt]+
				" WHERE "+quoted[idAt]+" = "+literals[idAt]+";\n")
			literals[deferredAt] = "NULL"
		}
		if _, err := out.WriteString(prefix + strings.Join(literals, ", ") + ");\n"); err != nil {
			return 0, nil, fmt.Errorf("write backup: %w", err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, nil, databaseError("read "+table, err)
	}
	return count, updates, nil
}

// sqliteTimeLayout is the layout the SQLite driver itself writes and parses.
const sqliteTimeLayout = "2006-01-02 15:04:05.999999999-07:00"

func sqlLiteral(driver string, value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "NULL", nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case int32:
		return strconv.FormatInt(int64(v), 10), nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return "", model.ErrInvalidInput
		}
		return strconv.FormatFloat(v, 'g', -1, 64), nil
	case bool:
		if v {
			return "TRUE", nil
		}
		return "FALSE", nil
	case time.Time:
		if driver == "sqlite" {
			return quoteString(v.Format(sqliteTimeLayout)), nil
		}
		return quoteString(v.Format(time.RFC3339Nano)), nil
	case []byte:
		// Binary asset columns are handled separately by dumpTable; other
		// bytes are text or JSON returned by the driver.
		if !utf8.Valid(v) {
			return "", model.ErrInvalidInput
		}
		return quoteString(string(v)), nil
	case string:
		return quoteString(v), nil
	case map[string]any, []any:
		// JSON columns decoded by the driver are written back as JSON text.
		encoded, err := json.Marshal(v)
		if err != nil {
			return "", model.ErrInvalidInput
		}
		return quoteString(string(encoded)), nil
	}
	return "", model.ErrInvalidInput
}

func quoteString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// Restore replaces every data table with the statements a Dump produced, in
// one transaction: either the whole backup lands or nothing changes. The
// target must already be migrated to the backup's schema version.
func (r *BackupRepository) Restore(ctx context.Context, driver string, schemaVersion int64, dump io.Reader) error {
	if driver != r.db.Name() {
		return fmt.Errorf("restore backup: %w", model.ErrBackupMismatch)
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current int64
		if err := tx.Raw("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&current).Error; err != nil {
			return err
		}
		if current != schemaVersion {
			return model.ErrBackupMismatch
		}
		if driver == "postgres" {
			// Dumped strings never escape backslashes; pin the setting that
			// makes them literal so the parse below matches the server's.
			if err := tx.Exec("SET LOCAL standard_conforming_strings = on").Error; err != nil {
				return err
			}
		}
		if err := tx.Exec(`UPDATE "albums" SET "cover_image_id" = NULL`).Error; err != nil {
			return err
		}
		for i := len(backupTables) - 1; i >= 0; i-- {
			if err := tx.Exec("DELETE FROM " + quoteIdent(backupTables[i])).Error; err != nil {
				return err
			}
		}
		statements := newStatementReader(dump)
		for {
			statement, err := statements.next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return err
			}
			if !allowedRestoreStatement(statement) {
				return model.ErrBackupMismatch
			}
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		if driver == "postgres" {
			// Explicit ids leave identity sequences behind; move each one past
			// the restored rows so the next insert does not collide.
			for _, table := range backupTables {
				var sequence *string
				if err := tx.Raw(`SELECT pg_get_serial_sequence(?, 'id') FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ? AND column_name = 'id'`, table, table).Scan(&sequence).Error; err != nil {
					return err
				}
				if sequence == nil || *sequence == "" {
					continue
				}
				if err := tx.Exec(`SELECT setval(?::regclass, GREATEST(COALESCE(MAX(id), 0), 1), MAX(id) IS NOT NULL) FROM `+quoteIdent(table), *sequence).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if errors.Is(err, model.ErrBackupMismatch) {
		return fmt.Errorf("restore backup: %w", model.ErrBackupMismatch)
	}
	if err != nil {
		return databaseError("restore backup", err)
	}
	return nil
}

// allowedRestoreStatement only lets the exact INSERTs and cover UPDATEs a
// Dump writes reach the database, so a tampered file cannot run anything
// else. The whole statement is parsed, not just its prefix: comments,
// escape-string syntax and any second statement fail the grammar, so the
// splitter and the database always agree on where a statement ends.
func allowedRestoreStatement(statement string) bool {
	p := &restoreParser{rest: statement}
	if p.take("INSERT INTO ") {
		table, ok := p.ident()
		if !ok || !slices.Contains(backupTables, table) || !p.take(" (") {
			return false
		}
		columns := 0
		for {
			if _, ok := p.ident(); !ok {
				return false
			}
			columns++
			if p.take(") VALUES (") {
				break
			}
			if !p.take(", ") {
				return false
			}
		}
		for i := range columns {
			if i > 0 && !p.take(", ") {
				return false
			}
			if !p.literal() {
				return false
			}
		}
		return p.take(")") && p.rest == ""
	}
	return p.take(`UPDATE "albums" SET "cover_image_id" = `) && p.integer() &&
		p.take(` WHERE "id" = `) && p.integer() && p.rest == ""
}

// restoreParser consumes the fixed dump grammar from the front of rest.
type restoreParser struct{ rest string }

func (p *restoreParser) take(token string) bool {
	if !strings.HasPrefix(p.rest, token) {
		return false
	}
	p.rest = p.rest[len(token):]
	return true
}

// ident reads a double-quoted lower-case identifier, the only kind the
// schema uses.
func (p *restoreParser) ident() (string, bool) {
	if !p.take(`"`) {
		return "", false
	}
	end := strings.IndexByte(p.rest, '"')
	if end <= 0 {
		return "", false
	}
	name := p.rest[:end]
	for _, char := range name {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' {
			return "", false
		}
	}
	p.rest = p.rest[end+1:]
	return name, true
}

// literal reads one value sqlLiteral writes: NULL, a boolean, a number or a
// standard single-quoted string with doubled quotes.
func (p *restoreParser) literal() bool {
	if p.take("NULL") || p.take("TRUE") || p.take("FALSE") {
		return true
	}
	if p.take("X'") {
		return p.hexBytes() && p.take("'")
	}
	if strings.HasPrefix(p.rest, "'\\x") {
		copy := *p
		copy.take("'\\x")
		if copy.hexBytes() && copy.take("'::bytea") {
			*p = copy
			return true
		}
	}
	if p.take("'") {
		for {
			end := strings.IndexByte(p.rest, '\'')
			if end < 0 {
				return false
			}
			p.rest = p.rest[end+1:]
			if !p.take("'") {
				return true
			}
		}
	}
	return p.number()
}

func (p *restoreParser) hexBytes() bool {
	end := strings.IndexByte(p.rest, '\'')
	if end < 0 {
		return false
	}
	if _, err := hex.DecodeString(p.rest[:end]); err != nil {
		return false
	}
	p.rest = p.rest[end:]
	return true
}

func (p *restoreParser) integer() bool {
	p.take("-")
	return p.digits()
}

func (p *restoreParser) number() bool {
	if !p.integer() {
		return false
	}
	if p.take(".") && !p.digits() {
		return false
	}
	if p.take("e") || p.take("E") {
		if !p.take("+") {
			p.take("-")
		}
		return p.digits()
	}
	return true
}

func (p *restoreParser) digits() bool {
	n := 0
	for n < len(p.rest) && p.rest[n] >= '0' && p.rest[n] <= '9' {
		n++
	}
	p.rest = p.rest[n:]
	return n > 0
}

// statementReader splits a dump on semicolons outside single-quoted strings
// and skips "--" comment lines between statements.
type statementReader struct{ in *bufio.Reader }

func newStatementReader(r io.Reader) *statementReader {
	return &statementReader{in: bufio.NewReaderSize(r, 64<<10)}
}

func (s *statementReader) next() (string, error) {
	var b strings.Builder
	quoted := false
	for {
		char, _, err := s.in.ReadRune()
		if errors.Is(err, io.EOF) {
			if strings.TrimSpace(b.String()) != "" {
				return "", model.ErrBackupMismatch
			}
			return "", io.EOF
		}
		if err != nil {
			return "", err
		}
		if !quoted && b.Len() == 0 {
			if char == '\n' || char == '\r' || char == ' ' || char == '\t' {
				continue
			}
			if char == '-' {
				if _, err := s.in.ReadString('\n'); err != nil && !errors.Is(err, io.EOF) {
					return "", err
				}
				continue
			}
		}
		if char == '\'' {
			quoted = !quoted
		}
		if char == ';' && !quoted {
			return b.String(), nil
		}
		b.WriteRune(char)
	}
}
