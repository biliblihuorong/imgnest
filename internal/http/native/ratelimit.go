package native

import (
	"sync"
	"time"
)

// fixedWindowLimiter counts requests per key in one-minute windows. Each
// instance owns its table, so flooding one route cannot exhaust the entries
// another route's limiter needs.
type fixedWindowLimiter struct {
	mu       sync.Mutex
	windows  map[string]window
	max      int
	capacity int
	now      func() time.Time
}

func newFixedWindowLimiter(max, capacity int, now func() time.Time) *fixedWindowLimiter {
	return &fixedWindowLimiter{windows: make(map[string]window), max: max, capacity: capacity, now: now}
}

// allow reports whether key may proceed in the current one-minute window. A
// new key is rejected while the table is full of unexpired keys.
func (l *fixedWindowLimiter) allow(key string) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, exists := l.windows[key]
	if !exists || !now.Before(entry.until) {
		for other, current := range l.windows {
			if !now.Before(current.until) {
				delete(l.windows, other)
			}
		}
		if len(l.windows) >= l.capacity {
			return false
		}
		entry = window{until: now.Add(time.Minute)}
	}
	entry.count++
	l.windows[key] = entry
	return entry.count <= l.max
}
