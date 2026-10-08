package migrate

import (
	"database/sql"
	"testing"
)

func TestOneGuestGroupMigrationKeepsConfiguredGroup(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *sql.DB, driver string) {
		scripts, err := manifest(driver)
		if err != nil {
			t.Fatal(err)
		}
		if err := up(t.Context(), db, driver, scripts[:6]); err != nil {
			t.Fatal(err)
		}
		// A creation race left three guest groups; the setting names the second.
		if _, err := db.ExecContext(t.Context(), `INSERT INTO groups (name, is_guest) VALUES ('guests-a', true), ('guests-b', true), ('guests-c', true)`); err != nil {
			t.Fatal(err)
		}
		point := `UPDATE settings SET value = (SELECT CAST(id AS TEXT) FROM groups WHERE name = 'guests-b') WHERE key = 'guest_group_id'`
		if driver == "postgres" {
			point = `UPDATE settings SET value = to_jsonb((SELECT id FROM groups WHERE name = 'guests-b')) WHERE key = 'guest_group_id'`
		}
		if _, err := db.ExecContext(t.Context(), point); err != nil {
			t.Fatal(err)
		}
		if err := Up(t.Context(), db, driver); err != nil {
			t.Fatal(err)
		}
		var kept string
		var flagged int
		if err := db.QueryRowContext(t.Context(), `SELECT MIN(name), COUNT(*) FROM groups WHERE is_guest = true`).Scan(&kept, &flagged); err != nil {
			t.Fatal(err)
		}
		if flagged != 1 || kept != "guests-b" {
			t.Fatalf("guest groups after migration = %d (%q), want only guests-b", flagged, kept)
		}
		if _, err := db.ExecContext(t.Context(), `INSERT INTO groups (name, is_guest) VALUES ('guests-d', true)`); err == nil {
			t.Fatal("second guest group accepted after migration")
		}
	})
}
