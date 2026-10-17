package cache

import (
	"os"
	"strings"
	"testing"
	"time"
)

// openTestRedis connects to IMGNEST_TEST_REDIS_URL or skips.
func openTestRedis(t *testing.T) *Redis {
	t.Helper()
	url := os.Getenv("IMGNEST_TEST_REDIS_URL")
	if url == "" {
		t.Skip("IMGNEST_TEST_REDIS_URL is not set")
	}
	store, err := OpenRedis(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestRedisRoundTrip(t *testing.T) {
	store := openTestRedis(t)
	ctx := t.Context()
	key := "test:" + t.Name() + ":" + time.Now().Format(time.RFC3339Nano)
	if _, ok, err := store.Get(ctx, key); err != nil || ok {
		t.Fatalf("fresh key = %v %v", ok, err)
	}
	if err := store.Set(ctx, key, []byte("value"), time.Minute); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Get(ctx, key)
	if err != nil || !ok || string(got) != "value" {
		t.Fatalf("get = %q %v %v", got, ok, err)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := store.Get(ctx, key); ok {
		t.Fatal("deleted key still present")
	}
	if err := store.Set(ctx, key, []byte("short"), 50*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(120 * time.Millisecond)
	if _, ok, _ := store.Get(ctx, key); ok {
		t.Fatal("key outlived its ttl")
	}
}

func TestOpenRedisHidesTheURL(t *testing.T) {
	_, err := OpenRedis(t.Context(), "redis://:hunter2@127.0.0.1:1/0")
	if err == nil {
		t.Fatal("connected to a closed port")
	}
	for _, secret := range []string{"hunter2", "127.0.0.1"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaks %q: %v", secret, err)
		}
	}
	if _, err := OpenRedis(t.Context(), "http://not-redis"); err == nil {
		t.Fatal("accepted a non-redis URL")
	}
}
