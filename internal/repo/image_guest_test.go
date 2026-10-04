package repo

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// guestFixture extends the standard image fixture with the guest group an
// anonymous upload resolves: the configured default is the is_guest fallback.
type guestFixture struct {
	imageFixture
	guests model.Group
}

// fixedRepoNow satisfies the trash check constraint in direct SQL updates.
var fixedRepoNow = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

func newGuestFixture(t *testing.T, db *gorm.DB, name string) guestFixture {
	t.Helper()
	inner := newImageFixture(t, db, name)
	guests := model.Group{Name: "Guests", IsGuest: true, CapacityBytes: 450, AllowedExts: []string{"jpg"}}
	if err := db.Create(&guests).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO group_policies (group_id, policy_id) VALUES (?, ?)", guests.ID, inner.policy.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE groups SET default_policy_id = ? WHERE id = ?", inner.policy.ID, guests.ID).Error; err != nil {
		t.Fatal(err)
	}
	return guestFixture{imageFixture: inner, guests: guests}
}

// guestRequest builds a guest reservation whose objects live on the given backend.
func guestRequest(storageID uint64, key, path string, size int64) model.UploadReservation {
	return model.UploadReservation{
		Image: model.Image{
			UserID: 0, StorageID: storageID, Key: key, Path: path, Ext: "jpg",
			MIME: "image/jpeg", OriginName: "anon.jpg", OperationID: "guest-" + key,
			HasOriginal: true, HasWebP: true, HasThumb: true,
			Size: size, WebPSize: 40, ThumbBytes: 20, Width: 10, Height: 10, Frames: 1,
		},
		Objects: []model.ObjectReceipt{
			{Key: path + ".jpg", Size: size, OwnerID: key, Location: model.ObjectLocationCloud, MIME: "image/jpeg"},
			{Key: path + ".webp", Size: 40, OwnerID: key, Location: model.ObjectLocationCloud, MIME: "image/webp"},
			{Key: path + "_thumbs.webp", Size: 20, OwnerID: key, Location: model.ObjectLocationCloud, MIME: "image/webp"},
			{Key: path + "_thumbs.webp", Size: 20, OwnerID: key, Location: model.ObjectLocationThumbCache, MIME: "image/webp"},
		},
	}
}

func TestGuestUploadPolicyResolution(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		policies, err := NewPolicyRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		// Without any guest group anonymous uploads are refused outright.
		if _, _, _, err := policies.GuestUploadPolicy(t.Context(), 0); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("guest policy without a guest group = %v, want forbidden", err)
		}

		fixture := newGuestFixture(t, db, "guest-policy")
		policy, backend, group, err := policies.GuestUploadPolicy(t.Context(), 0)
		if err != nil {
			t.Fatal(err)
		}
		if policy.ID != fixture.policy.ID || backend.ID != fixture.storage.ID || group.ID != fixture.guests.ID {
			t.Fatalf("guest policy = %d/%d/%d, want the granted default", policy.ID, backend.ID, group.ID)
		}

		// A policy outside the guest group's grants is refused.
		otherBackend, err := (func() (model.Storage, error) {
			backends, err := NewStorageRepository(t.Context(), db)
			if err != nil {
				return model.Storage{}, err
			}
			return backends.Create(t.Context(), model.Storage{Name: "second", Driver: "local", Enabled: true, BaseURL: "http://localhost/2", Config: json.RawMessage(`{}`)})
		})()
		if err != nil {
			t.Fatal(err)
		}
		stranger, err := policies.CreateAndBind(t.Context(), testPolicy(otherBackend.ID), fixture.user.GroupID, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := policies.GuestUploadPolicy(t.Context(), stranger.ID); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("unguested policy accepted: %v", err)
		}
	})
}

func TestReserveAndCommitGuestUpload(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture := newGuestFixture(t, db, "guest-upload")

		// Account reservations can never enter the guest pipeline.
		account := fixture.request("mix-1", "2026/01/mix-1")
		if _, err := fixture.images.ReserveGuestUpload(t.Context(), account); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("account reservation accepted by the guest path: %v", err)
		}

		// guest_group_id keeps its migration default of 0, so the is_guest
		// group is reached through the fallback path.
		reserved, err := fixture.images.ReserveGuestUpload(t.Context(), guestRequest(fixture.storage.ID, "guest-1", "2026/01/guest-1", 100))
		if err != nil {
			t.Fatal(err)
		}
		if reserved.UserID != 0 || reserved.State != model.ImageStatePending || reserved.Operation != model.ImageOperationUpload {
			t.Fatalf("reserved guest image = %+v, want a pending upload of the shared account", reserved)
		}
		if reserved.ChargedBytes != 160 { // 100 original + 40 webp + 20 thumb
			t.Fatalf("charged bytes = %d, want the cloud objects only", reserved.ChargedBytes)
		}

		used, err := fixture.images.GuestUsedBytes(t.Context())
		if err != nil || used != 160 {
			t.Fatalf("guest used bytes = %d err=%v, want the reservation counted", used, err)
		}

		// The same operation id resumes; a different one collides.
		replay := guestRequest(fixture.storage.ID, "guest-1", "2026/01/guest-1", 100)
		replay.Image.OperationID = reserved.OperationID
		if again, err := fixture.images.ReserveGuestUpload(t.Context(), replay); err != nil || again.ID != reserved.ID {
			t.Fatalf("resumable reservation failed: %v", err)
		}
		hijack := guestRequest(fixture.storage.ID, "guest-1", "2026/01/guest-1", 100)
		hijack.Image.OperationID = "guest-hijack-attempt"
		if _, err := fixture.images.ReserveGuestUpload(t.Context(), hijack); !errors.Is(err, model.ErrImageBusy) {
			t.Fatalf("path hijack accepted: %v", err)
		}

		// The guest group caps at 450 bytes: a second 160-byte upload fits,
		// and the 400-byte one must be refused before any object is written.
		if _, err := fixture.images.ReserveGuestUpload(t.Context(), guestRequest(fixture.storage.ID, "guest-2", "2026/01/guest-2", 160)); err != nil {
			t.Fatalf("second reservation refused: %v", err)
		}
		if _, err := fixture.images.ReserveGuestUpload(t.Context(), guestRequest(fixture.storage.ID, "guest-3", "2026/01/guest-3", 400)); !errors.Is(err, model.ErrQuotaExceeded) {
			t.Fatalf("guest quota not enforced: %v", err)
		}

		// Committing the wrong operation id is refused; the right one activates.
		if _, err := fixture.images.CommitGuestUpload(t.Context(), reserved.Key, "wrong-op", model.ImageExif{}); !errors.Is(err, model.ErrImageBusy) {
			t.Fatalf("wrong operation id accepted: %v", err)
		}
		committed, err := fixture.images.CommitGuestUpload(t.Context(), reserved.Key, reserved.OperationID, model.ImageExif{})
		if err != nil {
			t.Fatal(err)
		}
		if committed.State != model.ImageStateActive || committed.UserID != 0 || committed.Operation != "" {
			t.Fatalf("committed guest image = %+v", committed)
		}

		// Real user rows are untouched by guest traffic.
		var anchorUsed int64
		if err := db.Table("users").Select("used_bytes").Where("id = ?", 0).Scan(&anchorUsed).Error; err != nil {
			t.Fatal(err)
		}
		if anchorUsed != 0 {
			t.Fatalf("guest anchor charged bytes: %d", anchorUsed)
		}
	})
}

func TestGuestUsedBytesCountsReservedAndTrashRestore(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture := newGuestFixture(t, db, "guest-used")
		reserved, err := fixture.images.ReserveGuestUpload(t.Context(), guestRequest(fixture.storage.ID, "used-1", "2026/01/used-1", 100))
		if err != nil {
			t.Fatal(err)
		}
		used, err := fixture.images.GuestUsedBytes(t.Context())
		if err != nil || used != 160 {
			t.Fatalf("pending usage = %d err=%v, want 160", used, err)
		}
		if _, err := fixture.images.CommitGuestUpload(t.Context(), reserved.Key, reserved.OperationID, model.ImageExif{Raw: json.RawMessage(`{}`)}); err != nil {
			t.Fatal(err)
		}
		used, err = fixture.images.GuestUsedBytes(t.Context())
		if err != nil || used != 160 {
			t.Fatalf("active usage = %d err=%v, want 160", used, err)
		}
	})
}
