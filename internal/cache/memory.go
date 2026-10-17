// Package cache holds the small shared caches behind random links and the
// responsive-variant lookup: Redis when one is configured, otherwise memory
// in this process.
package cache

import (
	"context"
	"sync"
	"time"
)

// Store keeps short-lived byte values under string keys. A miss is
// (nil, false, nil); errors only report an unreachable backend, and callers
// treat them as a miss.
type Store interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
}

// DefaultMemoryEntries bounds a Memory store built with a zero limit.
const DefaultMemoryEntries = 16384

type memoryEntry struct {
	value []byte
	until time.Time
}

// Memory is the single-process Store. When full it first drops expired
// entries, then arbitrary ones, so a flood of distinct keys cannot grow it
// without bound.
type Memory struct {
	mu      sync.Mutex
	entries map[string]memoryEntry
	limit   int
	now     func() time.Time
}

// NewMemory creates an empty store holding at most limit entries (zero uses
// DefaultMemoryEntries); a nil clock uses the wall clock.
func NewMemory(limit int, now func() time.Time) *Memory {
	if limit <= 0 {
		limit = DefaultMemoryEntries
	}
	if now == nil {
		now = time.Now
	}
	return &Memory{entries: make(map[string]memoryEntry), limit: limit, now: now}
}

// Get returns a copy of the live value.
func (m *Memory) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.entries[key]
	if !ok {
		return nil, false, nil
	}
	if !m.now().Before(entry.until) {
		delete(m.entries, key)
		return nil, false, nil
	}
	return append([]byte(nil), entry.value...), true, nil
}

// Set stores a private copy of value for ttl; a non-positive ttl deletes it.
func (m *Memory) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	if ttl <= 0 {
		delete(m.entries, key)
		return nil
	}
	if _, exists := m.entries[key]; !exists && len(m.entries) >= m.limit {
		m.evict(now)
	}
	m.entries[key] = memoryEntry{value: append([]byte(nil), value...), until: now.Add(ttl)}
	return nil
}

// evict makes room for one entry: expired entries go first, and if none
// expired, one arbitrary entry is dropped.
func (m *Memory) evict(now time.Time) {
	for key, entry := range m.entries {
		if !now.Before(entry.until) {
			delete(m.entries, key)
		}
	}
	if len(m.entries) < m.limit {
		return
	}
	for key := range m.entries {
		delete(m.entries, key)
		return
	}
}

// Delete drops keys; unknown keys are not an error.
func (m *Memory) Delete(ctx context.Context, keys ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range keys {
		delete(m.entries, key)
	}
	return nil
}

// Len reports the stored entry count, expired ones included.
func (m *Memory) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries)
}
