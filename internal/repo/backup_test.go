package repo

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

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
		personal, err := NewPluginUserSettingsRepository(t.Context(), db, time.Now)
		if err != nil {
			t.Fatal(err)
		}
		logo := []byte{0x89, 0, 0xff, '\'', ';', '\\'}
		if err := personal.SaveAsset(t.Context(), "watermark", fixture.user.ID, fixture.user.GroupID, logo, "image/png"); err != nil {
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
		restored, mime, err := personal.Asset(t.Context(), "watermark", fixture.user.ID)
		if err != nil || mime != "image/png" || !bytes.Equal(restored, logo) {
			t.Fatalf("restored private logo = %x, %s, %v", restored, mime, err)
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

// A tampered dump must not smuggle a second statement past the allowlist by
// hiding the splitter's semicolon in syntax the database reads differently.
func TestBackupRestoreRejectsSmuggledStatements(t *testing.T) {
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
		payloads := map[string]string{
			"block comment": `INSERT INTO "settings" ("key", "value") VALUES ('poc', '{}') /*'*/; CREATE TABLE pwned (x INTEGER); /*'*/;`,
			"escape string": `INSERT INTO "settings" ("key", "value") VALUES (E'\'', '{}'); CREATE TABLE pwned (x INTEGER); --';`,
			"quoted alias":  `INSERT INTO "settings" ("key", "value") SELECT 'a' AS "x'y", '{}'; CREATE TABLE pwned (x INTEGER); --';`,
			"trailing":      `INSERT INTO "settings" ("key", "value") VALUES ('poc', '{}') RETURNING 1;`,
			"cover update":  `UPDATE "albums" SET "cover_image_id" = 1 WHERE "id" = 1 OR 1=1;`,
		}
		for name, payload := range payloads {
			err := backups.Restore(t.Context(), stats.Driver, stats.SchemaVersion, strings.NewReader(payload+"\n"))
			if !errors.Is(err, model.ErrBackupMismatch) {
				t.Errorf("%s: err = %v, want ErrBackupMismatch", name, err)
			}
		}
		var tables int64
		query := "SELECT COUNT(*) FROM sqlite_master WHERE name = 'pwned'"
		if db.Name() == "postgres" {
			query = "SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'pwned'"
		}
		if err := db.Raw(query).Scan(&tables).Error; err != nil {
			t.Fatal(err)
		}
		if tables != 0 {
			t.Fatal("smuggled statement created a table")
		}
	})
}

func TestAllowedRestoreStatementAcceptsDumpLiterals(t *testing.T) {
	for _, statement := range []string{
		`INSERT INTO "settings" ("key", "value") VALUES ('it''s; fine', '{"a":"b\\c"}')`,
		`INSERT INTO "images" ("id", "size", "ratio", "public", "album_id") VALUES (7, -12, 1.5e+21, TRUE, NULL)`,
		`UPDATE "albums" SET "cover_image_id" = 3 WHERE "id" = 9`,
	} {
		if !allowedRestoreStatement(statement) {
			t.Errorf("rejected dump statement %q", statement)
		}
	}
}
