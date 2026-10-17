package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUpdateCheck(t *testing.T) {
	cfg, err := Load(t.Context(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Updates.Check {
		t.Fatal("update checks are off by default")
	}
	cfg, err = Load(t.Context(), "", []string{"IMGNEST_UPDATES_CHECK=false"})
	if err != nil || cfg.Updates.Check {
		t.Fatalf("env false = %v, %v", cfg.Updates.Check, err)
	}
	if _, err := Load(t.Context(), "", []string{"IMGNEST_UPDATES_CHECK=sometimes"}); err == nil {
		t.Fatal("non-boolean update check accepted")
	}
	path := filepath.Join(t.TempDir(), "imgnest.yaml")
	if err := os.WriteFile(path, []byte("updates:\n  check: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(t.Context(), path, nil)
	if err != nil || cfg.Updates.Check {
		t.Fatalf("yaml false = %v, %v", cfg.Updates.Check, err)
	}
	if err := os.WriteFile(path, []byte("updates: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(t.Context(), path, nil); err == nil {
		t.Fatal("non-mapping updates section accepted")
	}
}
