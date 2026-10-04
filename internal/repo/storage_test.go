package repo

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func TestStorageConfigurationPersistence(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		r, err := NewStorageRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		input := model.Storage{
			Name: "local", Driver: "local", BaseURL: "http://localhost/files", Enabled: true,
			Config: json.RawMessage(`{"root":"data/images"}`),
		}
		created, err := r.Create(t.Context(), input)
		if err != nil {
			t.Fatal(err)
		}
		found, err := r.Find(t.Context(), created.ID)
		if err != nil || found.ID == 0 || found.CreatedAt.IsZero() {
			t.Fatalf("storage did not roundtrip: id=%d, error=%v", found.ID, err)
		}
		var config map[string]string
		if err := json.Unmarshal(found.Config, &config); err != nil || config["root"] != "data/images" {
			t.Fatal("backend configuration changed")
		}
		body, err := json.Marshal(found)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(body, []byte("data/images")) {
			t.Fatal("storage JSON exposed its configuration")
		}
		if err := db.WithContext(t.Context()).Model(&model.Storage{}).Where("id = ?", created.ID).Update("enabled", false).Error; err != nil {
			t.Fatal(err)
		}
		found, err = r.Find(t.Context(), created.ID)
		if err != nil || found.Enabled {
			t.Fatalf("disabled storage became unreadable: %v", err)
		}
		all, err := r.List(t.Context())
		if err != nil || len(all) != 1 {
			t.Fatalf("storage list count=%d error=%v", len(all), err)
		}
		if _, err := r.Create(t.Context(), model.Storage{Driver: "unknown", Config: json.RawMessage(`{}`)}); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("invalid backend error=%v, want ErrInvalidInput", err)
		}
	})
}
