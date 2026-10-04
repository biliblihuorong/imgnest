package service

import (
	"context"
	"sync"
	"testing"
	"time"
)

type pausedPreviewCache struct {
	ThumbCache
	started, release chan struct{}
	once             sync.Once
}

func (c *pausedPreviewCache) Put(ctx context.Context, id uint64, key string, data []byte) error {
	c.once.Do(func() { close(c.started) })
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.release:
		return c.ThumbCache.Put(ctx, id, key, data)
	}
}

func TestThumbnailRebuildCannotRacePathReassignment(t *testing.T) {
	svc, rows, _, _, subject := uploadFixture(t, "png")
	view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Trash(t.Context(), subject, view.Key); err != nil {
		t.Fatal(err)
	}
	if err = svc.deps.Cache.Delete(t.Context(), 1, "2026/10/a_thumbs.webp"); err != nil {
		t.Fatal(err)
	}
	cache := &pausedPreviewCache{ThumbCache: svc.deps.Cache, started: make(chan struct{}), release: make(chan struct{})}
	svc.deps.Cache = cache
	var release sync.Once
	unpause := func() { release.Do(func() { close(cache.release) }) }
	defer unpause()
	thumbDone := make(chan error, 1)
	go func() {
		object, err := svc.Thumbnail(t.Context(), subject, view.Key)
		if object.Body != nil {
			_ = object.Body.Close()
		}
		thumbDone <- err
	}()
	select {
	case <-cache.started:
	case <-time.After(2 * time.Second):
		t.Fatal("thumbnail rebuild not reached")
	}
	purgeDone := make(chan error, 1)
	go func() { purgeDone <- svc.Purge(t.Context(), subject, view.Key) }()
	select {
	case <-purgeDone:
		t.Fatal("purge released a path while an old preview could still write")
	case <-time.After(50 * time.Millisecond):
	}
	unpause()
	if err = <-thumbDone; err != nil {
		t.Fatal(err)
	}
	if err = <-purgeDone; err != nil {
		t.Fatal(err)
	}
	if len(rows.rows) != 0 {
		t.Fatal("purge failed after preview finished")
	}
	if _, err = svc.deps.Cache.Open(t.Context(), 1, "2026/10/a_thumbs.webp"); err == nil {
		t.Fatal("old preview reappeared after purge")
	}
}
