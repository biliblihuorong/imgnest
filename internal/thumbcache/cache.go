// Package thumbcache stores replaceable, ordinary WebP files below an os.Root.
package thumbcache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
)

// Cache is the uncharged local preview cache.
type Cache struct{ root *os.Root }

// New creates a confined cache directory.
func New(ctx context.Context, directory string) (*Cache, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("open thumbnail cache: %w", err)
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, fmt.Errorf("create thumbnail cache: %w", err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, fmt.Errorf("open thumbnail cache: %w", err)
	}
	return &Cache{root: root}, nil
}

// Close releases the directory descriptor.
func (c *Cache) Close() error { return c.root.Close() }

func cacheKey(storageID uint64, key string) (string, error) {
	if storageID == 0 || key == "" || path.Clean(key) != key || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") || key == ".." || strings.HasPrefix(key, "../") {
		return "", os.ErrInvalid
	}
	return strconv.FormatUint(storageID, 10) + "/" + key, nil
}

// Put atomically replaces a raw preview while the caller holds the image's path reservation.
func (c *Cache) Put(ctx context.Context, storageID uint64, key string, data []byte) (result error) {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("cache thumbnail: %w", err)
	}
	name, err := cacheKey(storageID, key)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return os.ErrInvalid
	}
	if err = c.root.MkdirAll(path.Dir(name), 0700); err != nil {
		return err
	}
	var random [12]byte
	if _, err = rand.Read(random[:]); err != nil {
		return err
	}
	temp := path.Dir(name) + "/.thumb-" + hex.EncodeToString(random[:])
	file, err := c.root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() {
		if err := c.root.Remove(temp); err != nil && !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, err)
		}
	}()
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return c.root.Rename(temp, name)
}

// Open returns only the raw cached bytes.
func (c *Cache) Open(ctx context.Context, storageID uint64, key string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	name, err := cacheKey(storageID, key)
	if err != nil {
		return nil, err
	}
	return c.root.Open(name)
}

// Delete is idempotent and never removes directories.
func (c *Cache) Delete(ctx context.Context, storageID uint64, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name, err := cacheKey(storageID, key)
	if err != nil {
		return err
	}
	err = c.root.Remove(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
