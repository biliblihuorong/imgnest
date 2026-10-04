package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/service"
	"gorm.io/gorm"
)

func TestInitLocalCLIConfiguresUsableDefaultPolicy(t *testing.T) {
	path := commandConfig(t)
	if _, err := command(t.Context(), t, path, "", "migrate"); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "objects")
	out, err := command(t.Context(), t, path, "", "init-local", "--root", root, "--base-url", "http://127.0.0.1:18080")
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Storage  service.StorageView `json:"storage"`
		PolicyID uint64              `json:"policy_id"`
	}
	if err = json.Unmarshal([]byte(out), &result); err != nil || result.Storage.ID == 0 || result.PolicyID == 0 {
		t.Fatalf("setup result: %v", err)
	}
	err = withDatabase(t.Context(), path, func(db *gorm.DB, _ config.Config) error {
		stores, err := repo.NewStorageRepository(t.Context(), db)
		if err != nil {
			return err
		}
		storage, err := stores.Find(t.Context(), result.Storage.ID)
		if err != nil {
			return err
		}
		if !strings.HasSuffix(storage.BaseURL, "/i/1") {
			t.Fatal("local URL route prefix missing")
		}
		var group model.Group
		if err = db.First(&group, "id = ?", 1).Error; err != nil {
			return err
		}
		if group.DefaultPolicyID != result.PolicyID {
			t.Fatal("default rule not bound")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStorageCLIRejectsSecretsInArgsAndMalformedStdin(t *testing.T) {
	path := commandConfig(t)
	if _, err := command(t.Context(), t, path, "", "migrate"); err != nil {
		t.Fatal(err)
	}
	if _, err := command(t.Context(), t, path, "", "init-storage", "--secret-access-key", "private-marker"); err == nil {
		t.Fatal("secret argument accepted")
	}
	if _, err := command(t.Context(), t, path, `{"config":"private-marker",broken`, "init-storage"); err == nil || strings.Contains(err.Error(), "private-marker") {
		t.Fatal("malformed config accepted or leaked")
	}
}
