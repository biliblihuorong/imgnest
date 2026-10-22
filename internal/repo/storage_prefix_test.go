package repo

import (
	"errors"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// An upload that resolved its driver before an empty storage moved to a new
// directory must not reserve against the old directory: its objects would be
// written where reads, restores and cleanup never look.
func TestReservationRejectsStalePathPrefix(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := newGuestFixture(t, db, "stale-prefix")
		backends, err := NewStorageRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		stale := f.storage
		moved := stale
		moved.PathPrefix = "new-folder"
		guard := model.StorageUpdateGuard{ExpectedDriver: stale.Driver, ExpectedConfig: stale.Config, ExpectedPathPrefix: stale.PathPrefix, RequireEmpty: true}
		if _, err := backends.Update(t.Context(), moved, guard); err != nil {
			t.Fatalf("move empty storage: %v", err)
		}

		account := f.request("account-1", "2026/01/account-1")
		account.Storage = stale
		if _, err := f.images.ReserveUpload(t.Context(), account); !errors.Is(err, model.ErrImageBusy) {
			t.Fatalf("account reservation with the old prefix: %v, want ErrImageBusy", err)
		}
		guest := guestRequest(stale.ID, "guest-1", "2026/01/guest-1", 100)
		guest.Storage = stale
		if _, err := f.images.ReserveGuestUpload(t.Context(), guest); !errors.Is(err, model.ErrImageBusy) {
			t.Fatalf("guest reservation with the old prefix: %v, want ErrImageBusy", err)
		}

		// The fresh snapshot is accepted.
		account.Storage.PathPrefix = "new-folder"
		if _, err := f.images.ReserveUpload(t.Context(), account); err != nil {
			t.Fatalf("account reservation with the current prefix: %v", err)
		}
		guest.Storage = account.Storage
		if _, err := f.images.ReserveGuestUpload(t.Context(), guest); err != nil {
			t.Fatalf("guest reservation with the current prefix: %v", err)
		}
	})
}
