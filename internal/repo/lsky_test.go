package repo

import (
	"errors"
	"strconv"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func TestAPIEnabledSwitch(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		lsky, err := NewLskyRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		// The migration default keeps the v1 API enabled.
		enabled, err := lsky.APIEnabled(t.Context())
		if err != nil || !enabled {
			t.Fatalf("default api_enabled = %v err=%v, want true", enabled, err)
		}
		if err := db.Exec("UPDATE settings SET value = 'false' WHERE key = 'api_enabled'").Error; err != nil {
			t.Fatal(err)
		}
		enabled, err = lsky.APIEnabled(t.Context())
		if err != nil || enabled {
			t.Fatalf("disabled api_enabled = %v err=%v, want false", enabled, err)
		}
		if err := db.Exec("UPDATE settings SET value = 'null' WHERE key = 'api_enabled'").Error; err != nil {
			t.Fatal(err)
		}
		enabled, err = lsky.APIEnabled(t.Context())
		if err != nil || !enabled {
			t.Fatalf("null api_enabled = %v err=%v, want the migration default true", enabled, err)
		}
		if err := db.Exec(`UPDATE settings SET value = '"yes"' WHERE key = 'api_enabled'`).Error; err != nil {
			t.Fatal(err)
		}
		if _, err = lsky.APIEnabled(t.Context()); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("invalid api_enabled accepted: %v", err)
		}
	})
}

func TestGuestUploadEnabledSwitch(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		lsky, err := NewLskyRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		// The migration default keeps anonymous uploads closed.
		enabled, err := lsky.GuestUploadEnabled(t.Context())
		if err != nil || enabled {
			t.Fatalf("default guest_upload_enabled = %v err=%v, want false", enabled, err)
		}
		if err := db.Exec("UPDATE settings SET value = 'true' WHERE key = 'guest_upload_enabled'").Error; err != nil {
			t.Fatal(err)
		}
		if enabled, err = lsky.GuestUploadEnabled(t.Context()); err != nil || !enabled {
			t.Fatalf("enabled guest_upload_enabled = %v err=%v, want true", enabled, err)
		}
		if err := db.Exec(`UPDATE settings SET value = '"true"' WHERE key = 'guest_upload_enabled'`).Error; err != nil {
			t.Fatal(err)
		}
		if _, err = lsky.GuestUploadEnabled(t.Context()); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("invalid guest_upload_enabled accepted: %v", err)
		}
	})
}

func TestGuestGroupResolutionPriority(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		lsky, err := NewLskyRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		// Migration defaults: guest_group_id = 0 and no is_guest group.
		if _, ok, err := lsky.GuestGroup(t.Context()); err != nil || ok {
			t.Fatalf("unconfigured site resolved a guest group (ok=%v err=%v), want none", ok, err)
		}

		fallback := model.Group{Name: "Guests", IsGuest: true, CapacityBytes: 4096, AllowedExts: []string{}}
		if err := db.Create(&fallback).Error; err != nil {
			t.Fatal(err)
		}
		group, ok, err := lsky.GuestGroup(t.Context())
		if err != nil || !ok || group.ID != fallback.ID || !group.IsGuest {
			t.Fatalf("is_guest fallback = %+v ok=%v err=%v, want the is_guest group", group, ok, err)
		}

		explicit := model.Group{Name: "Anonymous", IsGuest: false, CapacityBytes: 8192, AllowedExts: []string{}}
		if err := db.Create(&explicit).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("UPDATE settings SET value = '" + strconv.FormatUint(explicit.ID, 10) + "' WHERE key = 'guest_group_id'").Error; err != nil {
			t.Fatal(err)
		}
		group, ok, err = lsky.GuestGroup(t.Context())
		if err != nil || !ok || group.ID != explicit.ID || group.IsGuest {
			t.Fatalf("explicit guest_group_id = %+v ok=%v err=%v, want the configured group", group, ok, err)
		}

		// A dangling configured ID falls back to the is_guest group.
		if err := db.Exec("UPDATE settings SET value = '99999' WHERE key = 'guest_group_id'").Error; err != nil {
			t.Fatal(err)
		}
		group, ok, err = lsky.GuestGroup(t.Context())
		if err != nil || !ok || group.ID != fallback.ID {
			t.Fatalf("dangling guest_group_id = %+v ok=%v err=%v, want the is_guest fallback", group, ok, err)
		}
	})
}

func TestRegisteredIPAndOwnerCounters(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		lsky, err := NewLskyRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		fixture := newImageFixture(t, db, "lsky-counters")
		if err := db.Exec("UPDATE users SET registered_ip = '203.0.113.5' WHERE id = ?", fixture.user.ID).Error; err != nil {
			t.Fatal(err)
		}
		ip, err := lsky.RegisteredIP(t.Context(), fixture.user.ID)
		if err != nil || ip != "203.0.113.5" {
			t.Fatalf("registered_ip = %q err=%v", ip, err)
		}
		// The guest anchor row exists for the users foreign key; it simply
		// never registered from any address.
		if ip, err = lsky.RegisteredIP(t.Context(), 0); err != nil || ip != "" {
			t.Fatalf("guest anchor registered_ip = %q err=%v, want an empty address", ip, err)
		}

		reserved := fixture.request("cnt-1", "2026/01/cnt-1")
		if _, err := fixture.images.ReserveUpload(t.Context(), reserved); err != nil {
			t.Fatal(err)
		}
		active, err := lsky.CountActiveImages(t.Context(), fixture.user.ID)
		if err != nil || active != 0 {
			t.Fatalf("pending reservation counted as active: %d err=%v", active, err)
		}
		reserveAndCommit(t, fixture, "cnt-2", "2026/01/cnt-2")
		active, err = lsky.CountActiveImages(t.Context(), fixture.user.ID)
		if err != nil || active != 1 {
			t.Fatalf("active count = %d err=%v, want 1", active, err)
		}

		if capacity, err := lsky.GroupCapacityBytes(t.Context(), fixture.user.GroupID); err != nil || capacity != 0 {
			t.Fatalf("group capacity = %d err=%v, want the unlimited default 0", capacity, err)
		}
		if _, err := lsky.GroupCapacityBytes(t.Context(), 99999); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("missing group capacity accepted: %v", err)
		}
	})
}
