// Package randompool caches per-album redirect candidates.
package randompool

import (
	"context"
	"sync"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// sweepThreshold is the entry count above which a write also drops every
// expired album, bounding memory when many links are visited once.
const sweepThreshold = 1024

type entry struct {
	items []model.RandomCandidate
	until time.Time
}

// Memory is the single-process candidate pool. A shared implementation can
// replace it behind the same method set when several instances run.
type Memory struct {
	mu      sync.RWMutex
	entries map[uint64]entry
	now     func() time.Time
}

// NewMemory creates an empty pool; a nil clock uses the wall clock.
func NewMemory(now func() time.Time) *Memory {
	if now == nil {
		now = time.Now
	}
	return &Memory{entries: make(map[uint64]entry), now: now}
}

// Get returns a shared slice the caller must not modify.
func (m *Memory) Get(ctx context.Context, albumID uint64) ([]model.RandomCandidate, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	m.mu.RLock()
	cached, ok := m.entries[albumID]
	m.mu.RUnlock()
	if !ok || !m.now().Before(cached.until) {
		return nil, false, nil
	}
	return cached.items, true, nil
}

// Set stores a private copy of items, including an empty result, for ttl.
func (m *Memory) Set(ctx context.Context, albumID uint64, items []model.RandomCandidate, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now := m.now()
	stored := make([]model.RandomCandidate, len(items))
	copy(stored, items)
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.entries) >= sweepThreshold {
		for id, cached := range m.entries {
			if !now.Before(cached.until) {
				delete(m.entries, id)
			}
		}
	}
	m.entries[albumID] = entry{items: stored, until: now.Add(ttl)}
	return nil
}

// Invalidate drops one album's candidates; unknown albums are not an error.
func (m *Memory) Invalidate(ctx context.Context, albumID uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.entries, albumID)
	m.mu.Unlock()
	return nil
}

func (m *Memory) size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}
