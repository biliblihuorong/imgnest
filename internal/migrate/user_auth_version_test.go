package migrate

import (
	"database/sql"
	"testing"
)

func TestUserAuthVersionMigrationPreservesAccountsAndTokens(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *sql.DB, driver string) {
		scripts, err := manifest(driver)
		if err != nil {
			t.Fatal(err)
		}
		if err := up(t.Context(), db, driver, scripts[:5]); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), `INSERT INTO users
			(username,email,password_hash,display_name,role,group_id,status,used_bytes)
			VALUES ('existing','existing@example.com','digest','Existing Nick','user',1,'enabled',17)`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), `INSERT INTO tokens (user_id,name,token_hash,kind,abilities)
			SELECT id,'existing token','token-digest','api','["*"]' FROM users WHERE username='existing'`); err != nil {
			t.Fatal(err)
		}
		if err := Up(t.Context(), db, driver); err != nil {
			t.Fatal(err)
		}
		var epoch, used int64
		var display string
		if err := db.QueryRowContext(t.Context(), `SELECT auth_version, used_bytes, display_name FROM users WHERE username='existing'`).Scan(&epoch, &used, &display); err != nil {
			t.Fatal(err)
		}
		if epoch != 0 || used != 17 || display != "Existing Nick" {
			t.Fatalf("epoch=%d used=%d display=%q", epoch, used, display)
		}
		var tokens int
		if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM tokens WHERE name='existing token'`).Scan(&tokens); err != nil {
			t.Fatal(err)
		}
		if tokens != 1 {
			t.Fatalf("migration removed tokens: %d", tokens)
		}
		if err := Up(t.Context(), db, driver); err != nil {
			t.Fatal(err)
		}
	})
}
