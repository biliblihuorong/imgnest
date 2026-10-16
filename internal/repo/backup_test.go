package repo

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func TestBackupTablesCoverEveryMigratedTable(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		var tables []string
		query := `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`
		if db.Name() == "postgres" {
			query = `SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema() AND table_type = 'BASE TABLE'`
		}
		if err := db.Raw(query).Scan(&tables).Error; err != nil {
			t.Fatal(err)
		}
		for _, table := range tables {
			if table != "schema_migrations" && !slices.Contains(backupTables, table) {
				t.Errorf("table %s is missing from backupTables", table)
			}
		}
		for _, table := range backupTables {
			if !slices.Contains(tables, table) {
				t.Errorf("backupTables names %s, which no migration creates", table)
			}
		}
	})
}

func TestBackupDumpRestoreRoundTrip(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture := newImageFixture(t, db, "backup")
		albums, err := NewAlbumRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		album, err := albums.Create(t.Context(), model.Album{UserID: fixture.user.ID, Name: "O'Brien; -- 相册\n第二行"})
		if err != nil {
			t.Fatal(err)
		}
		image := reserveAndCommit(t, fixture, "backup-cover", "2026/01/backup-cover")
		if err := db.Exec("UPDATE images SET album_id = ? WHERE id = ?", album.ID, image.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("UPDATE albums SET cover_image_id = ? WHERE id = ?", image.ID, album.ID).Error; err != nil {
			t.Fatal(err)
		}
		settings, err := NewSettingsRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		if err := settings.UpdateSettings(t.Context(), map[string]json.RawMessage{"site_name": json.RawMessage(`"it's; mine"`)}); err != nil {
			t.Fatal(err)
		}
		backups, err := NewBackupRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		var first bytes.Buffer
		stats, err := backups.Dump(t.Context(), &first)
		if err != nil {
			t.Fatal(err)
		}
		if stats.Driver != db.Name() || stats.SchemaVersion == 0 || stats.Tables["images"] != 1 || stats.Tables["albums"] != 1 {
			t.Fatalf("stats = %+v", stats)
		}
		if !strings.Contains(first.String(), `UPDATE "albums" SET "cover_image_id"`) {
			t.Fatal("dump does not restore the album cover after its image")
		}

		// Changes made after the backup disappear on restore.
		if _, err := albums.Create(t.Context(), model.Album{UserID: fixture.user.ID, Name: "after backup"}); err != nil {
			t.Fatal(err)
		}
		if err := backups.Restore(t.Context(), stats.Driver, stats.SchemaVersion, bytes.NewReader(first.Bytes())); err != nil {
			t.Fatal(err)
		}
		var second bytes.Buffer
		if _, err := backups.Dump(t.Context(), &second); err != nil {
			t.Fatal(err)
		}
		if first.String() != second.String() {
			t.Fatalf("restored database differs from the backup\nfirst:\n%s\nsecond:\n%s", first.String(), second.String())
		}
		// Identity sequences moved past the restored ids.
		created, err := albums.Create(t.Context(), model.Album{UserID: fixture.user.ID, Name: "new after restore"})
		if err != nil {
			t.Fatalf("insert after restore: %v", err)
		}
		if created.ID <= album.ID {
			t.Fatalf("new album id %d does not follow restored id %d", created.ID, album.ID)
		}
	})
}

func TestBackupRestoreRejectsMismatchAndForeignStatements(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		backups, err := NewBackupRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		var dump bytes.Buffer
		stats, err := backups.Dump(t.Context(), &dump)
		if err != nil {
			t.Fatal(err)
		}
		other := "sqlite"
		if db.Name() == "sqlite" {
			other = "postgres"
		}
		cases := map[string]func() error{
			"driver": func() error {
				return backups.Restore(t.Context(), other, stats.SchemaVersion, bytes.NewReader(dump.Bytes()))
			},
			"schema": func() error {
				return backups.Restore(t.Context(), stats.Driver, stats.SchemaVersion+1, bytes.NewReader(dump.Bytes()))
			},
			"ddl": func() error {
				return backups.Restore(t.Context(), stats.Driver, stats.SchemaVersion, strings.NewReader("DROP TABLE users;\n"))
			},
			"unterminated": func() error {
				return backups.Restore(t.Context(), stats.Driver, stats.SchemaVersion, strings.NewReader(`INSERT INTO "settings" ("key") VALUES ('x')`))
			},
		}
		for name, restore := range cases {
			if err := restore(); !errors.Is(err, model.ErrBackupMismatch) {
				t.Errorf("%s: err = %v, want ErrBackupMismatch", name, err)
			}
		}
		var users int64
		if err := db.Raw("SELECT COUNT(*) FROM users").Scan(&users).Error; err != nil {
			t.Fatalf("users table after rejected restore: %v", err)
		}
	})
}
