package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxPathPrefix bounds a storage folder so prefixed keys stay well within
// the 1024-byte S3 key limit.
const MaxPathPrefix = 200

// ErrInvalidPrefix rejects a storage folder that is not a plain relative path.
var ErrInvalidPrefix = errors.New("invalid storage path prefix")

// NormalizePrefix trims surrounding spaces and slashes ("/a/b/" → "a/b") and
// checks that every segment is a plain name: no dot segments, no reserved
// "_trash"/"_checks" names, no backslashes or control characters.
func NormalizePrefix(prefix string) (string, error) {
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if prefix == "" {
		return "", nil
	}
	if !utf8.ValidString(prefix) || len(prefix) > MaxPathPrefix || strings.ContainsAny(prefix, "\\:") {
		return "", ErrInvalidPrefix
	}
	for _, part := range strings.Split(prefix, "/") {
		if part == "" || part == "." || part == ".." || part == "_trash" || part == "_checks" || part == ".trash" || strings.HasPrefix(part, ".imgnest-") || strings.TrimSpace(part) != part {
			return "", ErrInvalidPrefix
		}
		for _, r := range part {
			if unicode.IsControl(r) {
				return "", ErrInvalidPrefix
			}
		}
	}
	return prefix, nil
}

// WithPrefix places every key of driver under folder prefix, which must
// already be normalized. Callers keep using bare image keys; probes, the
// recycle bin and purges all stay inside the folder. An empty prefix returns
// driver unchanged.
func WithPrefix(driver Driver, prefix string) (Driver, error) {
	if prefix == "" {
		return driver, nil
	}
	if normalized, err := NormalizePrefix(prefix); err != nil || normalized != prefix {
		return nil, fmt.Errorf("storage folder %w", ErrInvalidPrefix)
	}
	base := &prefixed{inner: driver, prefix: prefix + "/"}
	if legacy, ok := driver.(LegacyObjects); ok {
		return &prefixedLegacy{prefixed: base, legacy: legacy}, nil
	}
	return base, nil
}

type prefixed struct {
	inner  Driver
	prefix string
}

func (p *prefixed) key(key string) string { return p.prefix + key }

func (p *prefixed) PutNew(ctx context.Context, key string, body io.ReadSeeker, opts PutOptions) (Receipt, error) {
	receipt, err := p.inner.PutNew(ctx, p.key(key), body, opts)
	receipt.Key = key
	return receipt, err
}

func (p *prefixed) Open(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error) {
	return p.inner.Open(ctx, p.key(key))
}

func (p *prefixed) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	return p.inner.Stat(ctx, p.key(key))
}

func (p *prefixed) Copy(ctx context.Context, source, target string, opts CopyOptions) (Receipt, error) {
	receipt, err := p.inner.Copy(ctx, p.key(source), p.key(target), opts)
	receipt.Key = target
	return receipt, err
}

func (p *prefixed) DeleteCurrent(ctx context.Context, key string) error {
	return p.inner.DeleteCurrent(ctx, p.key(key))
}

func (p *prefixed) PurgeAllVersions(ctx context.Context, key string) error {
	return p.inner.PurgeAllVersions(ctx, p.key(key))
}

// PurgeOwned implements OwnedPurger when the wrapped driver does.
func (p *prefixed) PurgeOwned(ctx context.Context, key, owner string) error {
	purger, ok := p.inner.(OwnedPurger)
	if !ok {
		return ErrUnsupported
	}
	return purger.PurgeOwned(ctx, p.key(key), owner)
}

// PurgeImage implements ImagePurger when the wrapped driver does.
func (p *prefixed) PurgeImage(ctx context.Context, key, owner string) error {
	purger, ok := p.inner.(ImagePurger)
	if !ok {
		return ErrUnsupported
	}
	return purger.PurgeImage(ctx, p.key(key), owner)
}

// Close releases the wrapped driver when it holds resources.
func (p *prefixed) Close() error {
	if closer, ok := p.inner.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

type prefixedLegacy struct {
	*prefixed
	legacy LegacyObjects
}

func (p *prefixedLegacy) CopyLegacy(ctx context.Context, source, version, target, digest string, opts CopyOptions) (Receipt, error) {
	receipt, err := p.legacy.CopyLegacy(ctx, p.key(source), version, p.key(target), digest, opts)
	receipt.Key = target
	return receipt, err
}

func (p *prefixedLegacy) DeleteLegacy(ctx context.Context, key, version string) error {
	return p.legacy.DeleteLegacy(ctx, p.key(key), version)
}

func (p *prefixedLegacy) PurgeLegacy(ctx context.Context, key, owner, version string) error {
	return p.legacy.PurgeLegacy(ctx, p.key(key), owner, version)
}
