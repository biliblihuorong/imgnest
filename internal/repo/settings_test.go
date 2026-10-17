package repo

import (
	"errors"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func TestTrashDaysSettings(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		_, settings := repositories(t, db)
		days, err := settings.TrashDays(t.Context())
		if err != nil || days != 7 {
			t.Fatalf("default trashdays=%d error=%v", days, err)
		}
		for _, tc := range []struct {
			name, value string
			want        int
			invalid     bool
		}{
			{"zero", "0", 0, false}, {"maximum", "36500", 36500, false}, {"negative", "-1", 0, true},
			{"overflow", "36501", 0, true}, {"fraction", "1.5", 0, true}, {"string", `"7"`, 0, true}, {"null", "null", 0, true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if err := db.WithContext(t.Context()).Exec("UPDATE settings SET value = ? WHERE key = 'trash_days'", tc.value).Error; err != nil {
					t.Fatal(err)
				}
				days, err := settings.TrashDays(t.Context())
				if tc.invalid {
					if !errors.Is(err, model.ErrInvalidInput) {
						t.Fatalf("invalid trashdays accepted: %v", err)
					}
					return
				}
				if err != nil || days != tc.want {
					t.Fatalf("trashdays=%d error=%v, want %d", days, err, tc.want)
				}
			})
		}
	})
}

func TestGuestUploadOpenNeedsSwitchAndGroup(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		_, settings := repositories(t, db)
		ctx := t.Context()
		if open, err := settings.GuestUploadOpen(ctx); err != nil || open {
			t.Fatalf("default = %v, %v; want closed", open, err)
		}
		if err := db.WithContext(ctx).Exec("UPDATE settings SET value = 'true' WHERE key = 'guest_upload_enabled'").Error; err != nil {
			t.Fatal(err)
		}
		var guests int64
		if err := db.WithContext(ctx).Table("groups").Where("is_guest = ?", true).Count(&guests).Error; err != nil {
			t.Fatal(err)
		}
		if guests == 0 {
			if open, err := settings.GuestUploadOpen(ctx); err != nil || open {
				t.Fatalf("switch on without a guest group = %v, %v; want closed", open, err)
			}
			if err := db.WithContext(ctx).Create(&model.Group{Name: "Guests", IsGuest: true, AllowedExts: []string{"png"}}).Error; err != nil {
				t.Fatal(err)
			}
		}
		if open, err := settings.GuestUploadOpen(ctx); err != nil || !open {
			t.Fatalf("switch on with a guest group = %v, %v; want open", open, err)
		}
	})
}
