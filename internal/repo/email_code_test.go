package repo

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func TestEmailCodes(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		codes, err := NewEmailCodeRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)
		hash := strings.Repeat("a", 64)
		if _, err := codes.FindEmailCode(t.Context(), model.EmailCodeRegister, "a@example.com"); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("missing code: %v", err)
		}
		first, err := codes.ReplaceEmailCode(t.Context(), model.EmailCode{Purpose: model.EmailCodeRegister,
			Email: "a@example.com", CodeHash: hash, ExpiresAt: now.Add(10 * time.Minute), CreatedAt: now})
		if err != nil || first.ID == 0 {
			t.Fatalf("store: %+v, %v", first, err)
		}
		if err := codes.AddEmailCodeAttempt(t.Context(), first.ID); err != nil {
			t.Fatal(err)
		}
		found, err := codes.FindEmailCode(t.Context(), model.EmailCodeRegister, "a@example.com")
		if err != nil || found.Attempts != 1 || !found.ExpiresAt.Equal(now.Add(10*time.Minute)) {
			t.Fatalf("find: %+v, %v", found, err)
		}
		// A newer request replaces the code and resets the attempts.
		newer := strings.Repeat("b", 64)
		second, err := codes.ReplaceEmailCode(t.Context(), model.EmailCode{Purpose: model.EmailCodeRegister,
			Email: "a@example.com", CodeHash: newer, ExpiresAt: now.Add(11 * time.Minute), CreatedAt: now.Add(time.Minute)})
		if err != nil || second.Attempts != 0 || second.CodeHash != newer {
			t.Fatalf("replace: %+v, %v", second, err)
		}
		if _, err := codes.ReplaceEmailCode(t.Context(), model.EmailCode{Purpose: model.EmailCodeResetPassword,
			Email: "a@example.com", CodeHash: hash, ExpiresAt: now.Add(-time.Minute), CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
		if deleted, err := codes.DeleteEmailCode(t.Context(), second.ID, hash); err != nil || deleted {
			t.Fatalf("stale hash deleted the new code: %v, %v", deleted, err)
		}
		if deleted, err := codes.DeleteEmailCode(t.Context(), second.ID, newer); err != nil || !deleted {
			t.Fatalf("delete: %v, %v", deleted, err)
		}
		if deleted, _ := codes.DeleteEmailCode(t.Context(), second.ID, newer); deleted {
			t.Fatal("a code was used twice")
		}
		if err := codes.PruneEmailCodes(t.Context(), now); err != nil {
			t.Fatal(err)
		}
		if _, err := codes.FindEmailCode(t.Context(), model.EmailCodeResetPassword, "a@example.com"); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("expired code survived pruning: %v", err)
		}
	})
}
