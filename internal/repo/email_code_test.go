package repo

import (
	"errors"
	"strings"
	"sync"
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
		if ok, err := codes.ReserveEmailCodeAttempt(t.Context(), first.ID, hash, 5, now); err != nil || !ok {
			t.Fatalf("reserve attempt: %v, %v", ok, err)
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

// Concurrent guesses at the last remaining attempt: exactly one may be
// compared, and stale, expired or replaced codes never reserve one.
func TestEmailCodeAttemptBudgetIsAtomic(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		codes, err := NewEmailCodeRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)
		hash := strings.Repeat("c", 64)
		code, err := codes.ReplaceEmailCode(t.Context(), model.EmailCode{Purpose: model.EmailCodeResetPassword,
			Email: "b@example.com", CodeHash: hash, ExpiresAt: now.Add(10 * time.Minute), CreatedAt: now})
		if err != nil {
			t.Fatal(err)
		}
		for range 4 {
			if ok, err := codes.ReserveEmailCodeAttempt(t.Context(), code.ID, hash, 5, now); err != nil || !ok {
				t.Fatalf("reserve: %v, %v", ok, err)
			}
		}
		var (
			wg      sync.WaitGroup
			mu      sync.Mutex
			granted int
		)
		for range 8 {
			wg.Go(func() {
				ok, err := codes.ReserveEmailCodeAttempt(t.Context(), code.ID, hash, 5, now)
				if err != nil {
					t.Error(err)
				}
				if ok {
					mu.Lock()
					granted++
					mu.Unlock()
				}
			})
		}
		wg.Wait()
		if granted != 1 {
			t.Fatalf("%d concurrent guesses passed the last attempt, want 1", granted)
		}
		found, _ := codes.FindEmailCode(t.Context(), model.EmailCodeResetPassword, "b@example.com")
		if found.Attempts != 5 {
			t.Fatalf("attempts = %d, want 5", found.Attempts)
		}
		// A refund frees one attempt for the same code only.
		if err := codes.RefundEmailCodeAttempt(t.Context(), code.ID, hash); err != nil {
			t.Fatal(err)
		}
		if ok, _ := codes.ReserveEmailCodeAttempt(t.Context(), code.ID, strings.Repeat("d", 64), 5, now); ok {
			t.Fatal("a replaced code reserved an attempt")
		}
		if ok, _ := codes.ReserveEmailCodeAttempt(t.Context(), code.ID, hash, 5, now.Add(10*time.Minute)); ok {
			t.Fatal("an expired code reserved an attempt")
		}
		if ok, _ := codes.ReserveEmailCodeAttempt(t.Context(), code.ID, hash, 5, now); !ok {
			t.Fatal("the refunded attempt was not available")
		}
	})
}
