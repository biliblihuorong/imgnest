package cli

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/repo"
)

func TestBackupCLICreatesAndRestoresArchive(t *testing.T) {
	path := commandConfig(t)
	dir := filepath.Join(t.TempDir(), "backup")
	t.Setenv("IMGNEST_BACKUP_DIR", dir)
	if _, err := command(t.Context(), t, path, "", "migrate"); err != nil {
		t.Fatal(err)
	}
	if _, err := command(t.Context(), t, path, "correct horse battery", "init-admin", "--username", "tester", "--email", "tester@example.com"); err != nil {
		t.Fatal(err)
	}
	out, err := command(t.Context(), t, path, "", "backup", "create")
	if err != nil || !strings.HasPrefix(out, "backup written: imgnest-") {
		t.Fatalf("backup create = %q, %v", out, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("backup dir entries = %v, %v", entries, err)
	}
	archivePath := filepath.Join(dir, entries[0].Name())
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, file := range archive.File {
		names[file.Name] = true
	}
	_ = archive.Close()
	for _, want := range []string{"manifest.json", "database.sql", "config/imgnest.yaml", "config/imgnest.env"} {
		if !names[want] {
			t.Errorf("archive lacks %s: %v", want, names)
		}
	}

	cfg, err := config.Load(t.Context(), path, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	db, err := repo.Open(t.Context(), cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer func() { _ = sqlDB.Close() }()
	if err := db.Exec("UPDATE users SET email = 'changed@example.com' WHERE username = 'tester'").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := command(t.Context(), t, path, "", "backup", "restore", archivePath); err == nil {
		t.Fatal("restore ran without --yes")
	}
	if out, err := command(t.Context(), t, path, "", "backup", "restore", "--yes", archivePath); err != nil || !strings.Contains(out, "restored") {
		t.Fatalf("restore = %q, %v", out, err)
	}
	var email string
	if err := db.Raw("SELECT email FROM users WHERE username = 'tester'").Scan(&email).Error; err != nil || email != "tester@example.com" {
		t.Fatalf("email after restore = %q, %v", email, err)
	}
}
