package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type pausedDeleteCache struct {
	ThumbCache
	started, release chan struct{}
	calls            atomic.Int32
}

func (c *pausedDeleteCache) Delete(ctx context.Context, id uint64, key string) error {
	if c.calls.Add(1) == 1 {
		close(c.started)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.release:
		}
	}
	return c.ThumbCache.Delete(ctx, id, key)
}

func TestUploadForegroundCleanupCannotRaceSweepPathRelease(t *testing.T) {
	svc, rows, _, local, subject := uploadFixture(t, "png")
	cache := &pausedDeleteCache{ThumbCache: svc.deps.Cache, started: make(chan struct{}), release: make(chan struct{})}
	svc.deps.Cache = cache
	svc.deps.Drivers = uploadDriverProvider{driver: &failingUploadDriver{Driver: local, failAt: 2}}
	var once sync.Once
	release := func() { once.Do(func() { close(cache.release) }) }
	defer release()
	uploadDone := make(chan error, 1)
	go func() {
		_, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
		uploadDone <- err
	}()
	select {
	case <-cache.started:
	case <-time.After(2 * time.Second):
		t.Fatal("foreground cleanup did not reach cache deletion")
	}
	sweepDone := make(chan error, 1)
	go func() { sweepDone <- svc.Sweep(t.Context()) }()
	select {
	case <-sweepDone:
		t.Fatal("sweep released the path before foreground cleanup ended")
	case <-time.After(50 * time.Millisecond):
	}
	release()
	if err := <-uploadDone; !errors.Is(err, ErrStorage) {
		t.Fatalf("upload failure: %v", err)
	}
	if err := <-sweepDone; err != nil {
		t.Fatal(err)
	}
	if len(rows.rows) != 0 {
		t.Fatal("completed cleanup retained path")
	}
}
