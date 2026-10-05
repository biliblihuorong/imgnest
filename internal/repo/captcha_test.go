package repo

import (
	"encoding/json"
	"sync"
	"testing"

	"gorm.io/gorm"
)

func TestCaptchaSnapshotCompareAndSwap(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		r, err := NewSettingsRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		old, err := r.ReadCaptcha(t.Context())
		if err != nil || old != nil {
			t.Fatalf("default=%s err=%v", old, err)
		}
		one := json.RawMessage(`{"version":1,"enabled":false,"active":null,"draft":null}`)
		ok, err := r.CompareAndSwapCaptcha(t.Context(), nil, one)
		if err != nil || !ok {
			t.Fatalf("create=%v %v", ok, err)
		}
		ok, err = r.CompareAndSwapCaptcha(t.Context(), nil, one)
		if err != nil || ok {
			t.Fatalf("stale creation=%v %v", ok, err)
		}
		old, err = r.ReadCaptcha(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		two := json.RawMessage(`{"version":2,"enabled":false,"active":null,"draft":null}`)
		var wg sync.WaitGroup
		results := make(chan bool, 2)
		errs := make(chan error, 2)
		for range 2 {
			wg.Go(func() { ok, err := r.CompareAndSwapCaptcha(t.Context(), old, two); results <- ok; errs <- err })
		}
		wg.Wait()
		close(results)
		close(errs)
		wins := 0
		for ok := range results {
			if ok {
				wins++
			}
		}
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		if wins != 1 {
			t.Fatalf("concurrent wins=%d", wins)
		}
		stored, err := r.ReadCaptcha(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		var state struct {
			Version int `json:"version"`
		}
		if json.Unmarshal(stored, &state) != nil || state.Version != 2 {
			t.Fatalf("stored=%s", stored)
		}
		if _, err := r.CompareAndSwapCaptcha(t.Context(), stored, json.RawMessage(`not json`)); err == nil {
			t.Fatal("invalid snapshot persisted")
		}
	})
}
