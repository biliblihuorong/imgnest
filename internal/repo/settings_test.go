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
