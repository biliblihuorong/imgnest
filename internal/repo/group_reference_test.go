package repo

import (
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func groupReferenceValue(id uint64) json.RawMessage {
	return json.RawMessage(strconv.FormatUint(id, 10))
}

// A service preflight can succeed before another administrator deletes the
// group. The write transaction must reject that now-stale reference itself.
func TestSettingsRejectDefaultGroupDeletedAfterPreflight(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		admin, _, settings, _, _, _, _ := newAdminRepositories(t, db)
		original, err := settings.DefaultGroupID(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		group, err := admin.CreateGroup(t.Context(), model.Group{Name: "new default"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := admin.FindGroup(t.Context(), group.ID); err != nil {
			t.Fatal(err)
		}
		if err := admin.DeleteGroup(t.Context(), group.ID); err != nil {
			t.Fatal(err)
		}
		err = settings.UpdateSettings(t.Context(), map[string]json.RawMessage{
			"default_group_id": groupReferenceValue(group.ID),
			"site_name":        json.RawMessage(`"must roll back"`),
		})
		if !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("stale default group write error = %v, want not found", err)
		}
		if current, err := settings.DefaultGroupID(t.Context()); err != nil || current != original {
			t.Fatalf("failed write changed default group: id=%d err=%v", current, err)
		}
		if name, err := settings.SiteName(t.Context()); err != nil || name != "ImgNest" {
			t.Fatalf("failed write changed site name: %q err=%v", name, err)
		}
	})
}

// The inverse service preflight can observe no reference immediately before
// a settings write commits. Deletion must recheck while holding the group lock.
func TestDeleteGroupRejectsReferenceAddedAfterPreflight(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		admin, _, settings, _, _, _, _ := newAdminRepositories(t, db)
		group, err := admin.CreateGroup(t.Context(), model.Group{Name: "chosen default"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		refs, err := admin.SettingGroupReferences(t.Context(), group.ID)
		if err != nil || len(refs) != 0 {
			t.Fatalf("preflight references=%v err=%v", refs, err)
		}
		if err := settings.UpdateSettings(t.Context(), map[string]json.RawMessage{
			"default_group_id": groupReferenceValue(group.ID),
		}); err != nil {
			t.Fatal(err)
		}
		if err := admin.DeleteGroup(t.Context(), group.ID); !errors.Is(err, model.ErrStillReferenced) {
			t.Fatalf("referenced group delete error = %v, want still referenced", err)
		}
		if current, err := settings.DefaultGroupID(t.Context()); err != nil || current != group.ID {
			t.Fatalf("default group became invalid: id=%d err=%v", current, err)
		}
	})
}

func TestGroupReferenceAssignmentAndDeletionCannotBothCommit(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		admin, _, settings, _, _, _, _ := newAdminRepositories(t, db)
		original, err := settings.DefaultGroupID(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		for attempt := range 12 {
			group, err := admin.CreateGroup(t.Context(), model.Group{Name: "racing default"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			assigned, deleted := make(chan error, 1), make(chan error, 1)
			go func() {
				<-start
				assigned <- settings.UpdateSettings(t.Context(), map[string]json.RawMessage{
					"default_group_id": groupReferenceValue(group.ID),
				})
			}()
			go func() {
				<-start
				deleted <- admin.DeleteGroup(t.Context(), group.ID)
			}()
			close(start)
			assignErr, deleteErr := <-assigned, <-deleted
			want := original
			switch {
			case assignErr == nil && errors.Is(deleteErr, model.ErrStillReferenced):
				want = group.ID
			case deleteErr == nil && errors.Is(assignErr, model.ErrNotFound):
			default:
				t.Fatalf("attempt %d: assignment=%v deletion=%v; want one committed operation and one business rejection", attempt, assignErr, deleteErr)
			}
			if current, err := settings.DefaultGroupID(t.Context()); err != nil || current != want {
				t.Fatalf("invalid group after concurrent writes: id=%d want=%d err=%v", current, want, err)
			}
			if err := settings.UpdateSettings(t.Context(), map[string]json.RawMessage{
				"default_group_id": groupReferenceValue(original),
			}); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func TestGroupReferenceSettingsValidateTypeAndAllowClearingGuest(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		admin, _, settings, _, _, _, _ := newAdminRepositories(t, db)
		ordinary, err := admin.CreateGroup(t.Context(), model.Group{Name: "ordinary"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		guest, err := admin.CreateGroup(t.Context(), model.Group{Name: "guest", IsGuest: true}, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			name, key string
			value     json.RawMessage
		}{
			{"guest default", "default_group_id", groupReferenceValue(guest.ID)},
			{"ordinary guest", "guest_group_id", groupReferenceValue(ordinary.ID)},
			{"zero default", "default_group_id", json.RawMessage(`0`)},
			{"null default", "default_group_id", json.RawMessage(`null`)},
			{"string default", "default_group_id", json.RawMessage(`"1"`)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if err := settings.UpdateSettings(t.Context(), map[string]json.RawMessage{tc.key: tc.value}); !errors.Is(err, model.ErrInvalidInput) {
					t.Fatalf("invalid group reference error=%v, want invalid input", err)
				}
			})
		}
		if err := settings.UpdateSettings(t.Context(), map[string]json.RawMessage{
			"default_group_id": groupReferenceValue(ordinary.ID),
			"guest_group_id":   groupReferenceValue(guest.ID),
		}); err != nil {
			t.Fatal(err)
		}
		if err := settings.UpdateSettings(t.Context(), map[string]json.RawMessage{"guest_group_id": json.RawMessage(`0`)}); err != nil {
			t.Fatalf("clear guest setting: %v", err)
		}
		var seed model.Group
		if err := db.First(&seed, "is_default = ?", true).Error; err != nil {
			t.Fatal(err)
		}
		if err := settings.UpdateSettings(t.Context(), map[string]json.RawMessage{"default_group_id": groupReferenceValue(seed.ID)}); err != nil {
			t.Fatal(err)
		}
		if err := admin.DeleteGroup(t.Context(), ordinary.ID); err != nil {
			t.Fatalf("group remained referenced after setting changed: %v", err)
		}
	})
}
