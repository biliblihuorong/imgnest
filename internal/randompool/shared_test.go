package randompool

import (
	"os"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/cache"
)

func sharedStores(t *testing.T) map[string]cache.Store {
	t.Helper()
	stores := map[string]cache.Store{"memory": cache.NewMemory(0, nil)}
	if url := os.Getenv("IMGNEST_TEST_REDIS_URL"); url != "" {
		redis, err := cache.OpenRedis(t.Context(), url)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = redis.Close() })
		stores["redis"] = redis
	}
	return stores
}

func TestSharedRoundTripAndInvalidate(t *testing.T) {
	for name, store := range sharedStores(t) {
		t.Run(name, func(t *testing.T) {
			// Two pools over one store behave like two instances.
			first, second := NewShared(store), NewShared(store)
			album := uint64(time.Now().UnixNano())
			if _, ok, err := first.Get(t.Context(), album); err != nil || ok {
				t.Fatalf("empty pool hit=%v err=%v", ok, err)
			}
			if err := first.Set(t.Context(), album, items("a", "b"), time.Minute); err != nil {
				t.Fatal(err)
			}
			got, ok, err := second.Get(t.Context(), album)
			if err != nil || !ok || len(got) != 2 || got[1].Path != "b" || !got[0].HasWebP || got[0].Ext != "jpg" {
				t.Fatalf("other instance read %+v ok=%v err=%v", got, ok, err)
			}
			if err := second.Invalidate(t.Context(), album); err != nil {
				t.Fatal(err)
			}
			if _, ok, _ := first.Get(t.Context(), album); ok {
				t.Fatal("invalidation on one instance did not reach the other")
			}
			if err := first.Set(t.Context(), album, nil, time.Minute); err != nil {
				t.Fatal(err)
			}
			empty, ok, err := second.Get(t.Context(), album)
			if err != nil || !ok || len(empty) != 0 {
				t.Fatalf("empty album = %+v ok=%v err=%v, want a cached empty result", empty, ok, err)
			}
			_ = first.Invalidate(t.Context(), album)
		})
	}
}

func TestSharedIgnoresUndecodableValues(t *testing.T) {
	store := cache.NewMemory(0, nil)
	if err := store.Set(t.Context(), sharedKey(9), []byte("not json"), time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := NewShared(store).Get(t.Context(), 9); ok || err != nil {
		t.Fatalf("garbage value = hit %v err %v, want a miss", ok, err)
	}
}
