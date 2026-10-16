package config

import "testing"

func TestLoadBackupDirectory(t *testing.T) {
	cfg, err := Load(t.Context(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backup.Dir != "data/backup" {
		t.Fatalf("default backup dir = %q", cfg.Backup.Dir)
	}
	cfg, err = Load(t.Context(), "", []string{"IMGNEST_BACKUP_DIR=/srv/imgnest-backup"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backup.Dir != "/srv/imgnest-backup" {
		t.Fatalf("env backup dir = %q", cfg.Backup.Dir)
	}
	if _, err := Load(t.Context(), "", []string{"IMGNEST_BACKUP_DIR= "}); err == nil {
		t.Fatal("blank backup dir accepted")
	}
}
