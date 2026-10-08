// Package ratelimit provides the bounded in-process fixed-window counters the
// HTTP layers use for login, token and upload throttling.
package ratelimit

import (
	"net/netip"
	"sync"
	"time"
)

// DefaultCapacity bounds the number of live counters kept in memory.
const DefaultCapacity = 4096

type entry struct {
	until time.Time
	count int
}

// Limiter keeps fixed-window counters per key. When the table is full it
// evicts the counter closest to expiry instead of refusing new keys, so a
// flood of distinct clients can never lock everyone else out.
type Limiter struct {
	mu       sync.Mutex
	now      func() time.Time
	window   time.Duration
	capacity int
	entries  map[string]entry
}

// New creates a limiter; a non-positive capacity uses DefaultCapacity.
func New(now func() time.Time, window time.Duration, capacity int) *Limiter {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Limiter{now: now, window: window, capacity: capacity, entries: make(map[string]entry)}
}

// Allow counts one attempt for key and reports whether it stays within limit.
// A non-positive limit means unlimited and records nothing.
func (l *Limiter) Allow(key string, limit int) bool {
	if limit <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	current := l.current(key)
	current.count++
	l.entries[key] = current
	return current.count <= limit
}

// Exceeded reports whether key already used up limit, without counting.
func (l *Limiter) Exceeded(key string, limit int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	current, ok := l.entries[key]
	return ok && l.now().Before(current.until) && current.count >= limit
}

// Reset forgets key, for example after a successful login.
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

func (l *Limiter) current(key string) entry {
	now := l.now()
	if existing, ok := l.entries[key]; ok && now.Before(existing.until) {
		return existing
	}
	delete(l.entries, key)
	if len(l.entries) >= l.capacity {
		l.evict(now)
	}
	return entry{until: now.Add(l.window)}
}

// evict drops expired counters, then the one expiring soonest if still full.
func (l *Limiter) evict(now time.Time) {
	oldestKey, oldest := "", time.Time{}
	for key, current := range l.entries {
		if !now.Before(current.until) {
			delete(l.entries, key)
			continue
		}
		if oldestKey == "" || current.until.Before(oldest) {
			oldestKey, oldest = key, current.until
		}
	}
	if len(l.entries) >= l.capacity && oldestKey != "" {
		delete(l.entries, oldestKey)
	}
}

// ClientKey groups IPv6 clients by their /64 prefix, the smallest block a
// single subscriber normally controls; IPv4 and unparsable values pass through.
func ClientKey(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil || addr.Is4() || addr.Is4In6() {
		return ip
	}
	prefix, err := addr.Prefix(64)
	if err != nil {
		return ip
	}
	return prefix.String()
}
