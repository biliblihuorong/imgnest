package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/biliblihuorong/imgnest/internal/searchquery"
)

type searchBackfillTable struct {
	table, id        string
	sources, targets []string
	normalize        func([]string) []string
}

var searchBackfills = []searchBackfillTable{
	{"images", "id", []string{"origin_name"}, []string{"filename_search"}, func(s []string) []string { return []string{searchquery.Normalize(s[0])} }},
	{"image_exif", "image_id", []string{"make", "model", "lens"}, []string{"camera_search", "lens_search"}, func(s []string) []string {
		return []string{searchquery.Normalize(strings.TrimSpace(s[0] + " " + s[1])), searchquery.Normalize(s[2])}
	}},
	{"albums", "id", []string{"name"}, []string{"name_nfc", "name_search"}, func(s []string) []string { return []string{searchquery.NFC(s[0]), searchquery.Normalize(s[0])} }},
}

func pendingPredicate(table searchBackfillTable) string {
	parts := []string{}
	for _, col := range table.targets {
		parts = append(parts, col+" IS NULL")
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

// BackfillSearch is invoked only by the explicit migrate command. Each batch
// commits at most 250 rows; NULL target columns are durable restart markers.
// Original filenames, EXIF and album names are never rewritten. A second run
// touches only unfinished rows, and cancellation preserves committed progress.
func BackfillSearch(ctx context.Context, db *sql.DB, driver string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if db == nil || (driver != "sqlite" && driver != "postgres") {
		return errors.New("invalid search backfill database")
	}
	for _, table := range searchBackfills {
		for {
			n, err := backfillSearchBatch(ctx, db, driver, table)
			if err != nil {
				return migrationError("backfill search metadata", err)
			}
			if n == 0 {
				break
			}
		}
	}
	return nil
}
func backfillSearchBatch(ctx context.Context, db *sql.DB, driver string, table searchBackfillTable) (int, error) {
	// #nosec G202 -- Table and column identifiers come only from searchBackfills, never request data.
	rows, err := db.QueryContext(ctx, "SELECT "+table.id+", "+strings.Join(table.sources, ", ")+" FROM "+table.table+" WHERE "+pendingPredicate(table)+" ORDER BY "+table.id+" LIMIT 250")
	if err != nil {
		return 0, err
	}
	type row struct {
		id     int64
		values []string
	}
	batch := []row{}
	for rows.Next() {
		var id int64
		source := make([]sql.NullString, len(table.sources))
		dest := []any{&id}
		for i := range source {
			dest = append(dest, &source[i])
		}
		if err = rows.Scan(dest...); err != nil {
			_ = rows.Close()
			return 0, err
		}
		values := []string{}
		for _, s := range source {
			values = append(values, s.String)
		}
		batch = append(batch, row{id, values})
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil || len(batch) == 0 {
		return 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	for _, row := range batch {
		values := table.normalize(row.values)
		args := []any{}
		set := []string{}
		placeholder := func() string {
			if driver == "postgres" {
				return fmt.Sprintf("$%d", len(args))
			}
			return "?"
		}
		for i, col := range table.targets {
			args = append(args, values[i])
			set = append(set, col+" = "+placeholder())
		}
		args = append(args, row.id)
		where := table.id + " = " + placeholder() + " AND " + pendingPredicate(table)
		// Protect against accidental concurrent edits even in maintenance mode.
		for i, col := range table.sources {
			args = append(args, row.values[i])
			where += " AND COALESCE(" + col + ", '') = " + placeholder()
		}
		// #nosec G202 -- Only fixed manifest identifiers are assembled; all metadata values are bound.
		if _, err = tx.ExecContext(ctx, "UPDATE "+table.table+" SET "+strings.Join(set, ", ")+" WHERE "+where, args...); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return len(batch), nil
}
func checkSearchBackfill(ctx context.Context, db *sql.DB) error {
	for _, table := range searchBackfills {
		var pending bool
		if err := db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM "+table.table+" WHERE "+pendingPredicate(table)+")").Scan(&pending); err != nil {
			return migrationError("check search metadata", err)
		}
		if pending {
			return errors.New("search metadata backfill is incomplete; run migrate")
		}
	}
	return nil
}
