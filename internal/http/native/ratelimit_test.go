package native

import (
	"strconv"
	"testing"
	"time"
)

func TestLimiterWindow(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	limiter := newFixedWindowLimiter(3, 16, func() time.Time { return now })
	for i := range 3 {
		if !limiter.allow("a") {
			t.Fatalf("request %d inside the limit was rejected", i+1)
		}
	}
	if limiter.allow("a") {
		t.Fatal("fourth request in one window was allowed")
	}
	if !limiter.allow("b") {
		t.Fatal("one key's limit blocked another key")
	}
	now = now.Add(59 * time.Second)
	if limiter.allow("a") {
		t.Fatal("window reopened before a minute passed")
	}
	now = now.Add(time.Second)
	if !limiter.allow("a") {
		t.Fatal("window did not reopen after a minute")
	}
}

func TestLimiterCapacity(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	limiter := newFixedWindowLimiter(3, 2, func() time.Time { return now })
	if !limiter.allow("a") || !limiter.allow("b") {
		t.Fatal("keys within capacity were rejected")
	}
	if limiter.allow("c") {
		t.Fatal("a key beyond capacity was admitted")
	}
	if !limiter.allow("a") {
		t.Fatal("a tracked key was rejected because the table is full")
	}
	now = now.Add(time.Minute)
	if !limiter.allow("c") {
		t.Fatal("expired keys were not reclaimed for a new key")
	}
}

func TestLimitersAreIndependent(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC) }
	flooded, quiet := newFixedWindowLimiter(1, 4, now), newFixedWindowLimiter(1, 4, now)
	for i := range 10 {
		flooded.allow(strconv.Itoa(i))
	}
	if !quiet.allow("login") {
		t.Fatal("flooding one limiter exhausted another")
	}
}
