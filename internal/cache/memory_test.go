package cache

import (
	"context"
	"sync"
	"testing"
	"time"
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

func TestMemoryExpiresAndCopies(t *testing.T) {
	clk := &clock{now: time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)}
	store := NewMemory(0, clk.Now)
	value := []byte("abc")
	if err := store.Set(t.Context(), "k", value, time.Minute); err != nil {
		t.Fatal(err)
	}
	value[0] = 'x'
	got, ok, err := store.Get(t.Context(), "k")
	if err != nil || !ok || string(got) != "abc" {
		t.Fatalf("get = %q %v %v", got, ok, err)
	}
	got[1] = 'y'
	again, _, _ := store.Get(t.Context(), "k")
	if string(again) != "abc" {
		t.Fatalf("store shares its bytes: %q", again)
	}
	clk.Advance(time.Minute)
	if _, ok, _ := store.Get(t.Context(), "k"); ok {
		t.Fatal("entry outlived its ttl")
	}
}

func TestMemoryDeleteAndZeroTTL(t *testing.T) {
	store := NewMemory(0, nil)
	_ = store.Set(t.Context(), "a", []byte("1"), time.Minute)
	_ = store.Set(t.Context(), "b", []byte("2"), time.Minute)
	if err := store.Delete(t.Context(), "a", "missing"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := store.Get(t.Context(), "a"); ok {
		t.Fatal("deleted key still cached")
	}
	if err := store.Set(t.Context(), "b", []byte("3"), 0); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := store.Get(t.Context(), "b"); ok {
		t.Fatal("zero ttl kept the key")
	}
}

func TestMemoryStaysBounded(t *testing.T) {
	clk := &clock{now: time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)}
	store := NewMemory(4, clk.Now)
	for _, key := range []string{"a", "b", "c", "d"} {
		_ = store.Set(t.Context(), key, []byte(key), time.Second)
	}
	clk.Advance(2 * time.Second)
	_ = store.Set(t.Context(), "e", []byte("e"), time.Minute)
	if store.Len() != 1 {
		t.Fatalf("expired entries kept: %d", store.Len())
	}
	for _, key := range []string{"f", "g", "h", "i", "j"} {
		_ = store.Set(t.Context(), key, []byte(key), time.Minute)
	}
	if store.Len() > 4 {
		t.Fatalf("store grew past its limit: %d", store.Len())
	}
	if _, ok, _ := store.Get(t.Context(), "j"); !ok {
		t.Fatal("newest entry was evicted")
	}
}

func TestMemoryCanceledContext(t *testing.T) {
	store := NewMemory(0, nil)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := store.Get(ctx, "k"); err == nil {
		t.Fatal("canceled get succeeded")
	}
}
