package randompool

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
)

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newClock() *clock { return &clock{now: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)} }

func items(paths ...string) []model.RandomCandidate {
	out := make([]model.RandomCandidate, 0, len(paths))
	for _, path := range paths {
		out = append(out, model.RandomCandidate{StorageID: 1, Path: path, Ext: "jpg", HasWebP: true, HasOriginal: true})
	}
	return out
}

func TestMemoryHitAndExpiry(t *testing.T) {
	clk := newClock()
	pool := NewMemory(clk.Now)
	if _, ok, err := pool.Get(t.Context(), 7); err != nil || ok {
		t.Fatalf("empty pool hit=%v err=%v", ok, err)
	}
	if err := pool.Set(t.Context(), 7, items("a", "b"), 60*time.Second); err != nil {
		t.Fatal(err)
	}
	got, ok, err := pool.Get(t.Context(), 7)
	if err != nil || !ok || len(got) != 2 || got[0].Path != "a" || got[1].Path != "b" {
		t.Fatalf("get = %+v ok=%v err=%v", got, ok, err)
	}
	clk.Advance(59 * time.Second)
	if _, ok, _ := pool.Get(t.Context(), 7); !ok {
		t.Fatal("entry expired one second early")
	}
	clk.Advance(time.Second)
	if _, ok, _ := pool.Get(t.Context(), 7); ok {
		t.Fatal("entry outlived its 60 second TTL")
	}
}

func TestMemoryInvalidate(t *testing.T) {
	pool := NewMemory(newClock().Now)
	if err := pool.Set(t.Context(), 7, items("a"), time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := pool.Set(t.Context(), 8, items("b"), time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := pool.Invalidate(t.Context(), 7); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := pool.Get(t.Context(), 7); ok {
		t.Fatal("invalidated album still cached")
	}
	if _, ok, _ := pool.Get(t.Context(), 8); !ok {
		t.Fatal("invalidation removed another album")
	}
	if err := pool.Invalidate(t.Context(), 999); err != nil {
		t.Fatalf("invalidating an unknown album: %v", err)
	}
}

func TestMemoryStoresEmpty(t *testing.T) {
	pool := NewMemory(newClock().Now)
	if err := pool.Set(t.Context(), 7, nil, time.Minute); err != nil {
		t.Fatal(err)
	}
	got, ok, err := pool.Get(t.Context(), 7)
	if err != nil || !ok || len(got) != 0 {
		t.Fatalf("empty album get = %+v ok=%v err=%v, want a cached empty result", got, ok, err)
	}
}

func TestMemorySetCopies(t *testing.T) {
	pool := NewMemory(newClock().Now)
	source := items("a", "b")
	if err := pool.Set(t.Context(), 7, source, time.Minute); err != nil {
		t.Fatal(err)
	}
	source[0].Path = "mutated"
	got, _, _ := pool.Get(t.Context(), 7)
	if got[0].Path != "a" {
		t.Fatalf("pool shares the caller's slice: %+v", got)
	}
}

func TestMemorySweepsExpired(t *testing.T) {
	clk := newClock()
	pool := NewMemory(clk.Now)
	for id := uint64(1); id <= 2000; id++ {
		if err := pool.Set(t.Context(), id, items("a"), time.Second); err != nil {
			t.Fatal(err)
		}
	}
	clk.Advance(time.Minute)
	if err := pool.Set(t.Context(), 5000, items("fresh"), time.Minute); err != nil {
		t.Fatal(err)
	}
	if size := pool.size(); size != 1 {
		t.Fatalf("pool kept %d entries after expiry, want 1", size)
	}
}

func TestMemoryConcurrent(t *testing.T) {
	pool := NewMemory(time.Now)
	var wg sync.WaitGroup
	for worker := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 200 {
				album := uint64(i%5 + 1)
				switch (worker + i) % 3 {
				case 0:
					_ = pool.Set(context.Background(), album, items("a", "b"), time.Minute)
				case 1:
					got, _, _ := pool.Get(context.Background(), album)
					for _, item := range got {
						_ = item.Path
					}
				default:
					_ = pool.Invalidate(context.Background(), album)
				}
			}
		}()
	}
	wg.Wait()
}

func TestMemoryContextCancelled(t *testing.T) {
	pool := NewMemory(newClock().Now)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := pool.Get(ctx, 7); !errors.Is(err, context.Canceled) {
		t.Fatalf("get = %v", err)
	}
	if err := pool.Set(ctx, 7, items("a"), time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("set = %v", err)
	}
	if err := pool.Invalidate(ctx, 7); !errors.Is(err, context.Canceled) {
		t.Fatalf("invalidate = %v", err)
	}
}
