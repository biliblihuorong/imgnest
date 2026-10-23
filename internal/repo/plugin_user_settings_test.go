package repo

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func TestPluginUserSettingsIsolationAndMergedUpdates(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		users, settings := repositories(t, db)
		group, _ := settings.DefaultGroupID(t.Context())
		alice, err := users.CreateUser(t.Context(), testUser("alice", "alice@example.com", group))
		if err != nil {
			t.Fatal(err)
		}
		bob, err := users.CreateUser(t.Context(), testUser("bob", "bob@example.com", group))
		if err != nil {
			t.Fatal(err)
		}
		store, err := NewPluginUserSettingsRepository(t.Context(), db, time.Now)
		if err != nil {
			t.Fatal(err)
		}
		initial, err := store.Read(t.Context(), "watermark", alice.ID)
		if err != nil || initial.Version != 0 || string(initial.Values) != "{}" || initial.User.GroupID != group || initial.User.GroupName != "Default" {
			t.Fatalf("initial=%+v err=%v", initial, err)
		}
		if err := store.SaveValues(t.Context(), "watermark", alice.ID, group, json.RawMessage(`{"text":"Alice"}`)); err != nil {
			t.Fatal(err)
		}
		if err := store.SaveAsset(t.Context(), "watermark", alice.ID, group, []byte("logo"), "image/png"); err != nil {
			t.Fatal(err)
		}
		if err := store.SaveValues(t.Context(), "watermark", alice.ID, group, json.RawMessage(`{"text":"Updated"}`)); err != nil {
			t.Fatal(err)
		}
		current, err := store.Read(t.Context(), "watermark", alice.ID)
		if err != nil || current.Version != 3 || string(current.Values) != `{"text":"Updated"}` {
			t.Fatalf("current=%+v err=%v", current, err)
		}
		asset, mime, err := store.Asset(t.Context(), "watermark", alice.ID)
		if err != nil || !bytes.Equal(asset, []byte("logo")) || mime != "image/png" {
			t.Fatalf("asset=%q mime=%q err=%v", asset, mime, err)
		}
		for _, tc := range []struct {
			name, plugin string
			user         uint64
		}{{"other user", "watermark", bob.ID}, {"other plugin", "another", alice.ID}} {
			t.Run(tc.name, func(t *testing.T) {
				got, err := store.Read(t.Context(), tc.plugin, tc.user)
				if err != nil || got.Version != 0 || string(got.Values) != "{}" {
					t.Fatalf("leaked settings=%+v err=%v", got, err)
				}
				blob, _, err := store.Asset(t.Context(), tc.plugin, tc.user)
				if err != nil || len(blob) != 0 {
					t.Fatalf("leaked asset=%q err=%v", blob, err)
				}
			})
		}
		if err := store.SaveAsset(t.Context(), "watermark", alice.ID, group, nil, ""); err != nil {
			t.Fatal(err)
		}
		current, _ = store.Read(t.Context(), "watermark", alice.ID)
		asset, _, _ = store.Asset(t.Context(), "watermark", alice.ID)
		if len(asset) != 0 || current.Version != 4 || string(current.Values) != `{"text":"Updated"}` {
			t.Fatal("removal changed settings or kept asset")
		}
	})
}

func TestPluginUserSettingsRejectGroupChangesAndDisabledAccounts(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		users, settings := repositories(t, db)
		group, _ := settings.DefaultGroupID(t.Context())
		user, err := users.CreateUser(t.Context(), testUser("owner", "owner@example.com", group))
		if err != nil {
			t.Fatal(err)
		}
		store, err := NewPluginUserSettingsRepository(t.Context(), db, time.Now)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SaveValues(t.Context(), "watermark", user.ID, group+1, json.RawMessage(`{}`)); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("stale group=%v", err)
		}
		if err := store.SaveAsset(t.Context(), "watermark", user.ID, group+1, []byte("logo"), "image/png"); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("stale asset group=%v", err)
		}
		if err := db.Model(&model.User{}).Where("id = ?", user.ID).Update("status", model.UserStatusDisabled).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := store.Read(t.Context(), "watermark", user.ID); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("disabled read=%v", err)
		}
		if err := store.SaveValues(t.Context(), "watermark", user.ID, group, json.RawMessage(`{}`)); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("disabled write=%v", err)
		}
		if _, err := store.Read(t.Context(), "watermark", user.ID+99); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("missing=%v", err)
		}
	})
}
