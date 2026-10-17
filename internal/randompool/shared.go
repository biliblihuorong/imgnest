package randompool

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/biliblihuorong/imgnest/internal/cache"
	"github.com/biliblihuorong/imgnest/internal/model"
)

// Shared keeps candidates in a cache every instance reads (Redis), so an
// invalidation on one instance takes effect on all of them at once.
type Shared struct {
	store cache.Store
}

// NewShared wraps a shared byte store.
func NewShared(store cache.Store) *Shared {
	return &Shared{store: store}
}

func sharedKey(albumID uint64) string {
	return "random:album:" + strconv.FormatUint(albumID, 10)
}

// Get returns the album's candidates; a value that no longer decodes is a
// miss, so a format change never breaks redirects.
func (s *Shared) Get(ctx context.Context, albumID uint64) ([]model.RandomCandidate, bool, error) {
	raw, ok, err := s.store.Get(ctx, sharedKey(albumID))
	if err != nil || !ok {
		return nil, false, err
	}
	var items []model.RandomCandidate
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, false, nil
	}
	return items, true, nil
}

// Set stores items, including an empty result, for ttl.
func (s *Shared) Set(ctx context.Context, albumID uint64, items []model.RandomCandidate, ttl time.Duration) error {
	if items == nil {
		items = []model.RandomCandidate{}
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("encode random candidates: %w", err)
	}
	return s.store.Set(ctx, sharedKey(albumID), raw, ttl)
}

// Invalidate drops one album's candidates; unknown albums are not an error.
func (s *Shared) Invalidate(ctx context.Context, albumID uint64) error {
	return s.store.Delete(ctx, sharedKey(albumID))
}
