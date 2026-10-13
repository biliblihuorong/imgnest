package repo

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func TestPluginCounters(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		now := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
		counters, err := NewPluginCounterRepository(db, func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		if got, err := counters.Get(t.Context(), "quota", "uploads:2026-10", "7"); err != nil || got != 0 {
			t.Fatalf("missing counter = %d, %v", got, err)
		}
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				if _, err := counters.Add(t.Context(), "quota", "uploads:2026-10", "7", 100); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
		total, err := counters.Add(t.Context(), "quota", "uploads:2026-10", "7", -50)
		if err != nil || total != 750 {
			t.Fatalf("total = %d, %v", total, err)
		}
		if got, _ := counters.Get(t.Context(), "other", "uploads:2026-10", "7"); got != 0 {
			t.Fatalf("counters leak across plugins: %d", got)
		}
		if _, err := counters.Add(t.Context(), "quota", "", "7", 1); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("empty name accepted: %v", err)
		}
		now = now.Add(401 * 24 * time.Hour)
		if _, err := counters.Add(t.Context(), "quota", "uploads:2027-11", "7", 1); err != nil {
			t.Fatal(err)
		}
		pruned, err := counters.Prune(t.Context())
		if err != nil || pruned != 1 {
			t.Fatalf("pruned = %d, %v", pruned, err)
		}
		if got, _ := counters.Get(t.Context(), "quota", "uploads:2027-11", "7"); got != 1 {
			t.Fatalf("recent counter pruned: %d", got)
		}
	})
}
