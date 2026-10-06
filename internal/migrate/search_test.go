package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
)

func TestSearchBackfillResumesAndKeepsOriginals(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *sql.DB, driver string) {
		for _, q := range []string{"CREATE TABLE images(id BIGINT PRIMARY KEY, origin_name TEXT NOT NULL, filename_search TEXT)", "CREATE TABLE image_exif(image_id BIGINT PRIMARY KEY, make TEXT, model TEXT, lens TEXT, camera_search TEXT, lens_search TEXT)", "CREATE TABLE albums(id BIGINT PRIMARY KEY, name TEXT NOT NULL, name_nfc TEXT, name_search TEXT)"} {
			if _, err := db.Exec(q); err != nil {
				t.Fatal(err)
			}
		}
		for i := 1; i <= 503; i++ {
			q := "INSERT INTO images(id,origin_name) VALUES (?,?)"
			if driver == "postgres" {
				q = "INSERT INTO images(id,origin_name) VALUES ($1,$2)"
			}
			if _, err := db.Exec(q, i, "Cafe\u0301.JPG"); err != nil {
				t.Fatal(err)
			}
		}
		for _, q := range []string{"INSERT INTO image_exif(image_id,make,model,lens) VALUES (1,'Canon','EOS R6','RF24-105')", "INSERT INTO albums(id,name) VALUES (1,'Café')"} {
			if _, err := db.Exec(q); err != nil {
				t.Fatal(err)
			}
		}
		cancelled, cancel := context.WithCancel(t.Context())
		cancel()
		if err := BackfillSearch(cancelled, db, driver); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled: %v", err)
		}
		// A committed first batch represents an interrupted prior invocation.
		if _, err := db.Exec("UPDATE images SET filename_search = 'café.jpg' WHERE id <= 250"); err != nil {
			t.Fatal(err)
		}
		if err := BackfillSearch(t.Context(), db, driver); err != nil {
			t.Fatal(err)
		}
		if err := BackfillSearch(t.Context(), db, driver); err != nil {
			t.Fatal(err)
		}
		for _, v := range []struct{ query, want string }{{"SELECT origin_name FROM images WHERE id=503", "Cafe\u0301.JPG"}, {"SELECT filename_search FROM images WHERE id=503", "café.jpg"}, {"SELECT camera_search FROM image_exif WHERE image_id=1", "canon eos r6"}, {"SELECT lens_search FROM image_exif WHERE image_id=1", "rf24-105"}, {"SELECT name_nfc FROM albums WHERE id=1", "Café"}, {"SELECT name_search FROM albums WHERE id=1", "café"}} {
			var got string
			if err := db.QueryRow(v.query).Scan(&got); err != nil || got != v.want {
				t.Errorf("%s=%q want=%q err=%v", v.query, got, v.want, err)
			}
		}
		if err := checkSearchBackfill(t.Context(), db); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("UPDATE images SET filename_search=NULL WHERE id=1"); err != nil {
			t.Fatal(err)
		}
		if err := checkSearchBackfill(t.Context(), db); err == nil {
			t.Fatal("incomplete search data was considered ready")
		}
		_ = fmt.Sprint(driver)
	})
}
