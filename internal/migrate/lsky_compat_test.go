package migrate

import (
	"database/sql"
	"testing"
)

// publishedChecksums pins the published 0001/0002 scripts byte-for-byte so a
// later migration can never rewrite history.
var publishedChecksums = map[string]map[int64]string{
	"sqlite": {
		1: "90ef153625f486a1b7a808373a0f09523f24aa9083c0d67f3c400e8c21fc3248",
		2: "c5c3797fb9bd4b6fd203ccda3546f0c0e4b0216a0502d29304cb787381d169fd",
	},
	"postgres": {
		1: "e78d55e4dc05286d199428f59fbd9d42a806bdcb5fe212c3123da8aa70d24b19",
		2: "fe179ee10f1d450d4ab2c02e4bc71675db965ed14b525d114da25f6c7d6a0553",
	},
}

func Test0003AddsSettingsKeysAndRegisteredIP(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *sql.DB, driver string) {
		if err := Up(t.Context(), db, driver); err != nil {
			t.Fatal(err)
		}
		for key, want := range map[string]string{
			"api_enabled":    "true",
			"site_name":      `"ImgNest"`,
			"guest_group_id": "0",
		} {
			var value string
			if err := db.QueryRowContext(t.Context(), "SELECT value FROM settings WHERE key = $1", key).Scan(&value); err != nil {
				t.Fatalf("setting %s unavailable: %v", key, err)
			}
			if value != want {
				t.Fatalf("setting %s = %s, want %s", key, value, want)
			}
		}
		// The guest anchor row keeps images.user_id = 0 valid under the users
		// foreign key and proves the registered_ip column exists with its
		// empty default.
		var ip, status, role string
		var usedBytes int64
		if err := db.QueryRowContext(t.Context(),
			"SELECT registered_ip, status, role, used_bytes FROM users WHERE id = 0",
		).Scan(&ip, &status, &role, &usedBytes); err != nil {
			t.Fatalf("guest anchor row unavailable (registered_ip column missing?): %v", err)
		}
		if ip != "" || status != "disabled" || role != "user" || usedBytes != 0 {
			t.Fatalf("guest anchor row ip=%q status=%q role=%q used=%d", ip, status, role, usedBytes)
		}
		if err := Check(t.Context(), db, driver); err != nil {
			t.Fatal(err)
		}
	})
}

func Test0003IsIdempotent(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *sql.DB, driver string) {
		for range 2 {
			if err := Up(t.Context(), db, driver); err != nil {
				t.Fatal(err)
			}
		}
		var settings, anchors, versions int
		if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM settings").Scan(&settings); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM users WHERE id = 0").Scan(&anchors); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM schema_migrations").Scan(&versions); err != nil {
			t.Fatal(err)
		}
		if settings != 8 || anchors != 1 || versions != 4 {
			t.Fatalf("settings=%d anchors=%d versions=%d, want 8/1/4", settings, anchors, versions)
		}
	})
}

func Test0003PreservesPublishedChecksums(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *sql.DB, driver string) {
		scripts, err := manifest(driver)
		if err != nil {
			t.Fatal(err)
		}
		expected := publishedChecksums[driver]
		seen := 0
		for _, script := range scripts {
			want, pinned := expected[script.version]
			if !pinned {
				continue
			}
			seen++
			if script.checksum() != want {
				t.Fatalf("migration %04d checksum %s changed; published history must stay untouched", script.version, script.checksum())
			}
			var stored string
			if err := db.QueryRowContext(t.Context(),
				"SELECT checksum FROM schema_migrations WHERE version = $1", script.version,
			).Scan(&stored); err == nil && stored != want {
				t.Fatalf("applied checksum %s for migration %04d differs from the pinned %s", stored, script.version, want)
			}
		}
		if seen != len(expected) {
			t.Fatalf("manifest exposed %d pinned versions, want %d", seen, len(expected))
		}
	})
}
