package thumbcache

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestRawCache(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	cache, err := New(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cache.Close(); err != nil {
			t.Error(err)
		}
	}()
	data := []byte("RIFF raw webp bytes")
	if err = cache.Put(ctx, 2, "2026/图_thumbs.webp", data); err != nil {
		t.Fatal(err)
	}
	// #nosec G304 -- test-owned temporary directory and a literal fixture path.
	disk, err := os.ReadFile(filepath.Join(root, "2", "2026", "图_thumbs.webp"))
	if err != nil || string(disk) != string(data) {
		t.Fatal("cache is not raw bytes")
	}
	file, err := cache.Open(ctx, 2, "2026/图_thumbs.webp")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil || string(got) != string(data) {
		t.Fatal("read differs")
	}
	if err = cache.Put(ctx, 2, "2026/图_thumbs.webp", []byte("replacement")); err != nil {
		t.Fatal(err)
	}
	if err = cache.Put(ctx, 2, "../../escape.webp", data); err == nil {
		t.Fatal("cache escaped")
	}
	if err = cache.Delete(ctx, 2, "2026/图_thumbs.webp"); err != nil {
		t.Fatal(err)
	}
	if err = cache.Delete(ctx, 2, "2026/图_thumbs.webp"); err != nil {
		t.Fatal(err)
	}
	if _, err = cache.Open(ctx, 2, "2026/图_thumbs.webp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("not missing: %v", err)
	}
}
