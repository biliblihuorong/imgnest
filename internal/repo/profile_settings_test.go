package repo

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func TestUpdateDisplayNameVerbatim(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		users, settings := repositories(t, db)
		groupID, err := settings.DefaultGroupID(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		created, err := users.CreateUser(t.Context(), testUser("profile-user", "profile@example.com", groupID))
		if err != nil {
			t.Fatal(err)
		}
		if created.DisplayName != "" {
			t.Fatalf("new account display name=%q, want empty", created.DisplayName)
		}
		// The repository stores the value verbatim; trimming is service policy.
		updated, err := users.UpdateDisplayName(t.Context(), created.ID, "  Nick  ")
		if err != nil {
			t.Fatal(err)
		}
		if updated.DisplayName != "  Nick  " {
			t.Fatalf("display name=%q, want stored verbatim", updated.DisplayName)
		}
		reread, err := users.FindUserByID(t.Context(), created.ID)
		if err != nil || reread.DisplayName != "  Nick  " {
			t.Fatalf("reread display name=%q error=%v", reread.DisplayName, err)
		}
		cleared, err := users.UpdateDisplayName(t.Context(), created.ID, "")
		if err != nil || cleared.DisplayName != "" {
			t.Fatalf("clear display name=%q error=%v", cleared.DisplayName, err)
		}
		if _, err := users.UpdateDisplayName(t.Context(), created.ID+99, "x"); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("missing user error=%v, want ErrNotFound", err)
		}
	})
}

func TestAvatarConfigDefaultsValuesAndCorruption(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		_, settings := repositories(t, db)
		config, err := settings.AvatarConfig(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if config.Provider != model.DefaultAvatarProvider || config.Version != 0 {
			t.Fatalf("default config=%+v", config)
		}
		if err := settings.UpdateSettings(t.Context(), map[string]json.RawMessage{
			"avatar_provider": json.RawMessage(`"gravatar"`),
		}); err != nil {
			t.Fatal(err)
		}
		config, err = settings.AvatarConfig(t.Context())
		if err != nil || config.Provider != model.AvatarProviderGravatar {
			t.Fatalf("stored config=%+v error=%v", config, err)
		}
		if config.Version == 0 {
			t.Fatal("stored provider must expose a nonzero configuration version")
		}
		// Unparseable JSON cannot even be stored: both databases reject it
		// (SQLite json_valid CHECK, PostgreSQL jsonb), so only well-formed
		// values with unsupported content need the fallback here.
		for _, value := range []string{`"qq"`, `""`, `"weavatar "`, `null`} {
			if err := db.WithContext(t.Context()).Exec(
				"UPDATE settings SET value = ? WHERE key = 'avatar_provider'", value,
			).Error; err != nil {
				t.Fatal(err)
			}
			config, err := settings.AvatarConfig(t.Context())
			if err != nil {
				t.Fatalf("value %s errored: %v", value, err)
			}
			if config.Provider != model.DefaultAvatarProvider {
				t.Fatalf("value %s yielded provider=%q, want default", value, config.Provider)
			}
		}
	})
}
