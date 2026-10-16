package repo

import (
	"errors"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func TestAdminResetUserPasswordRevokesTokens(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		admin, owner, target, _ := managedAccountFixture(t, db)
		accountTokens(t, db, target)
		accountTokens(t, db, owner)
		if err := admin.ResetUserPassword(t.Context(), owner.ID, target.ID, "new-hash"); err != nil {
			t.Fatal(err)
		}
		var stored model.User
		if err := db.First(&stored, "id = ?", target.ID).Error; err != nil {
			t.Fatal(err)
		}
		if stored.PasswordHash != "new-hash" || stored.Status != target.Status || stored.Role != target.Role {
			t.Fatalf("reset changed more than the password: %+v", stored)
		}
		var targetTokens, ownerTokens int64
		db.Model(&model.Token{}).Where("user_id = ?", target.ID).Count(&targetTokens)
		db.Model(&model.Token{}).Where("user_id = ?", owner.ID).Count(&ownerTokens)
		if targetTokens != 0 || ownerTokens != 2 {
			t.Fatalf("tokens after reset: target=%d owner=%d", targetTokens, ownerTokens)
		}
		if err := admin.ResetUserPassword(t.Context(), owner.ID, owner.ID, "x"); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("self reset = %v", err)
		}
		if err := admin.ResetUserPassword(t.Context(), target.ID, owner.ID, "x"); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("non-admin reset = %v", err)
		}
		if err := admin.ResetUserPassword(t.Context(), owner.ID, 987654, "x"); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("missing target = %v", err)
		}
	})
}
