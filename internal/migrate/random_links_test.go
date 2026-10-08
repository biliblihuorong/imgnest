package migrate

import (
	"database/sql"
	"testing"
)

func TestRandomLinksMigration(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *sql.DB, driver string) {
		scripts, err := manifest(driver)
		if err != nil {
			t.Fatal(err)
		}
		if err := up(t.Context(), db, driver, scripts[:6]); err != nil {
			t.Fatal(err)
		}
		exec := func(query string) error {
			_, err := db.ExecContext(t.Context(), query)
			return err
		}
		user := func(name string) string {
			return `INSERT INTO users (username,email,password_hash,role,group_id,status)
				VALUES ('` + name + `','` + name + `@example.com','digest','user',1,'enabled')`
		}
		if err := exec(user("existing")); err != nil {
			t.Fatal(err)
		}
		if err := Up(t.Context(), db, driver); err != nil {
			t.Fatal(err)
		}
		var missing int
		if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM users WHERE username='existing' AND public_id IS NULL`).Scan(&missing); err != nil {
			t.Fatal(err)
		}
		if missing != 1 {
			t.Fatalf("existing account kept=%d, want one row without a public id", missing)
		}
		// Unset public ids never collide; assigned ones are unique.
		if err := exec(user("second")); err != nil {
			t.Fatalf("second account without public id: %v", err)
		}
		if err := exec(`UPDATE users SET public_id='aaaaaaaaaa' WHERE username='existing'`); err != nil {
			t.Fatal(err)
		}
		if err := exec(`UPDATE users SET public_id='aaaaaaaaaa' WHERE username='second'`); err == nil {
			t.Fatal("duplicate public id was accepted")
		}
		for _, name := range []string{"one", "two"} {
			if err := exec(`INSERT INTO albums (user_id,name) SELECT id,'` + name + `' FROM users WHERE username='existing'`); err != nil {
				t.Fatal(err)
			}
		}
		link := func(album, token string) string {
			return `INSERT INTO random_links (user_id,album_id,token)
				SELECT user_id,id,'` + token + `' FROM albums WHERE name='` + album + `'`
		}
		if err := exec(link("one", "token-one")); err != nil {
			t.Fatal(err)
		}
		if err := exec(link("one", "token-other")); err == nil {
			t.Fatal("second link for one album was accepted")
		}
		if err := exec(link("two", "token-one")); err == nil {
			t.Fatal("duplicate token was accepted")
		}
		var enabled bool
		if err := db.QueryRowContext(t.Context(), `SELECT enabled FROM random_links WHERE token='token-one'`).Scan(&enabled); err != nil {
			t.Fatal(err)
		}
		if !enabled {
			t.Fatal("links must default to enabled")
		}
		// The raw album rows above need the search backfill a rerun performs.
		if err := Up(t.Context(), db, driver); err != nil {
			t.Fatal(err)
		}
		if err := Check(t.Context(), db, driver); err != nil {
			t.Fatal(err)
		}
	})
}
