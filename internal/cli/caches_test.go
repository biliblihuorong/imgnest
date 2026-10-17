package cli

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/cache"
	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/randompool"
)

func TestOpenCachesFallsBackToMemory(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	caches, closeCaches, err := openCaches(t.Context(), config.Cache{}, logger)
	if err != nil {
		t.Fatal(err)
	}
	closeCaches()
	if _, ok := caches.pool.(*randompool.Memory); !ok {
		t.Fatalf("pool = %T, want memory", caches.pool)
	}
	caches, closeCaches, err = openCaches(t.Context(), config.Cache{RedisURL: "redis://:hunter2@127.0.0.1:1/0"}, logger)
	if err != nil {
		t.Fatal(err)
	}
	closeCaches()
	if _, ok := caches.lookup.(*cache.Memory); !ok {
		t.Fatalf("unreachable redis did not fall back to memory: %T", caches.lookup)
	}
	if strings.Contains(logs.String(), "hunter2") || !strings.Contains(logs.String(), "redis unavailable") {
		t.Fatalf("fallback log: %s", logs.String())
	}
}

func TestOpenCachesUsesRedis(t *testing.T) {
	url := os.Getenv("IMGNEST_TEST_REDIS_URL")
	if url == "" {
		t.Skip("IMGNEST_TEST_REDIS_URL is not set")
	}
	caches, closeCaches, err := openCaches(t.Context(), config.Cache{RedisURL: url}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer closeCaches()
	if _, ok := caches.pool.(*randompool.Shared); !ok {
		t.Fatalf("pool = %T, want the shared Redis pool", caches.pool)
	}
	if _, ok := caches.lookup.(*cache.Redis); !ok {
		t.Fatalf("lookup = %T, want Redis", caches.lookup)
	}
}
