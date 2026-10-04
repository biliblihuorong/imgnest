package storage

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Local stores atomic owned objects beneath a filesystem root.
type Local struct{ root *os.Root }

// NewLocal opens a filesystem object root.
func NewLocal(ctx context.Context, dir string) (*Local, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, safeError("create object root", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, safeError("open object root", err)
	}
	return &Local{root: root}, nil
}

// Close releases the root descriptor.
func (d *Local) Close() error { return d.root.Close() }

// PutNew installs a new object without replacing any existing key.
func (d *Local) PutNew(ctx context.Context, key string, body io.ReadSeeker, opts PutOptions) (receipt Receipt, result error) {
	if err := d.validate(ctx, key); err != nil {
		return receipt, err
	}
	if err := validateOptions(opts); err != nil {
		return receipt, err
	}
	if body == nil {
		return receipt, fmt.Errorf("object body is required")
	}
	if _, err := d.root.Lstat(key); err == nil {
		return receipt, ErrExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return receipt, safeError("inspect object target", err)
	}
	size, err := body.Seek(0, io.SeekEnd)
	if err != nil || size < 0 {
		return receipt, safeError("measure object", err)
	}
	if _, err := body.Seek(0, io.SeekStart); err != nil {
		return receipt, safeError("rewind object", err)
	}
	info := ObjectInfo{Size: size, MIME: opts.MIME, OwnerID: opts.OwnerID}
	header, err := json.Marshal(info)
	if err != nil {
		return receipt, safeError("encode object metadata", err)
	}
	headerLength := len(header)
	if headerLength < 0 || headerLength > 16384 {
		return receipt, fmt.Errorf("object metadata exceeds header limit")
	}
	if err := d.root.MkdirAll(path.Dir(key), 0700); err != nil {
		return receipt, safeError("create object directory", err)
	}
	temporary := localStagingName(key, opts.OwnerID)
	file, err := d.root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return receipt, ErrExists
		}
		return receipt, safeError("create temporary object", err)
	}
	defer func() {
		if err := d.root.Remove(temporary); err != nil && !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, safeError("remove temporary object", err))
		}
	}()
	closed := false
	defer func() {
		if !closed {
			if err := file.Close(); err != nil {
				result = errors.Join(result, safeError("close temporary object", err))
			}
		}
	}()
	prefix := make([]byte, 12)
	copy(prefix, localMagic)
	binary.BigEndian.PutUint32(prefix[8:], uint32(headerLength))
	if _, err := file.Write(prefix); err != nil {
		return receipt, safeError("write object header", err)
	}
	if _, err := file.Write(header); err != nil {
		return receipt, safeError("write object metadata", err)
	}
	n, err := io.Copy(file, contextReader{ctx, body})
	if err != nil {
		return receipt, safeError("write object content", err)
	}
	if n != size {
		return receipt, fmt.Errorf("object body length changed")
	}
	if err := file.Sync(); err != nil {
		return receipt, safeError("sync object", err)
	}
	err = file.Close()
	closed = true
	if err != nil {
		return receipt, safeError("close object", err)
	}
	if err := ctx.Err(); err != nil {
		return receipt, err
	}
	if err := d.root.Link(temporary, key); err != nil {
		if errors.Is(err, os.ErrExist) {
			return receipt, ErrExists
		}
		return receipt, safeError("install object", err)
	}
	receipt = Receipt{Key: key, Size: size, OwnerID: opts.OwnerID}
	directory, err := d.root.Open(path.Dir(key))
	if err != nil {
		return receipt, safeError("open object directory", err)
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil || closeErr != nil {
		return receipt, safeError("sync object directory", errors.Join(syncErr, closeErr))
	}
	return receipt, nil
}

// Open reads the original object bytes.
func (d *Local) Open(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error) {
	file, info, err := d.openFile(ctx, key)
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	return &localReader{contextReader: contextReader{ctx, file}, file: file}, info, nil
}

// Stat reads durable ownership metadata.
func (d *Local) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	file, info, err := d.openFile(ctx, key)
	if err != nil {
		return ObjectInfo{}, err
	}
	if err := file.Close(); err != nil {
		return ObjectInfo{}, safeError("close object", err)
	}
	return info, nil
}

// Copy creates a target or verifies an identical owned retry.
func (d *Local) Copy(ctx context.Context, source, target string, opts CopyOptions) (Receipt, error) {
	file, info, err := d.openFile(ctx, source)
	if err != nil {
		return Receipt{}, err
	}
	defer func() { _ = file.Close() }()
	if info.OwnerID != opts.OwnerID {
		return Receipt{}, ErrOwnership
	}
	offset, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return Receipt{}, safeError("locate source content", err)
	}
	if opts.MIME == "" {
		opts.MIME = info.MIME
	}
	receipt, err := d.PutNew(ctx, target, io.NewSectionReader(file, offset, info.Size), opts)
	if !errors.Is(err, ErrExists) {
		return receipt, err
	}
	existing, existingInfo, err := d.openFile(ctx, target)
	if err != nil {
		return Receipt{}, err
	}
	defer func() { _ = existing.Close() }()
	if existingInfo.OwnerID != opts.OwnerID {
		return Receipt{}, ErrOwnership
	}
	if existingInfo.Size != info.Size {
		return Receipt{}, ErrExists
	}
	sourceDigest, targetDigest := sha256.New(), sha256.New()
	if _, err := io.Copy(sourceDigest, contextReader{ctx, io.NewSectionReader(file, offset, info.Size)}); err != nil {
		return Receipt{}, safeError("compare copy source", err)
	}
	if _, err := io.Copy(targetDigest, contextReader{ctx, existing}); err != nil {
		return Receipt{}, safeError("compare copy target", err)
	}
	if !strings.EqualFold(hex.EncodeToString(sourceDigest.Sum(nil)), hex.EncodeToString(targetDigest.Sum(nil))) {
		return Receipt{}, ErrExists
	}
	return Receipt{Key: target, Size: info.Size, OwnerID: opts.OwnerID}, nil
}

// DeleteCurrent removes one owned object.
func (d *Local) DeleteCurrent(ctx context.Context, key string) error {
	if _, err := d.Stat(ctx, key); errors.Is(err, ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := d.root.Remove(key); err != nil && !errors.Is(err, os.ErrNotExist) {
		return safeError("delete object", err)
	}
	return nil
}

// PurgeAllVersions removes exactly one local key.
func (d *Local) PurgeAllVersions(ctx context.Context, key string) error {
	return d.DeleteCurrent(ctx, key)
}

const localMagic = "IMGNST01"

// PurgeImage removes an exclusively owned image object and its staging file.
func (d *Local) PurgeImage(ctx context.Context, key, owner string) error {
	if err := d.PurgeOwned(ctx, key, owner); err != nil {
		return err
	}
	if _, err := d.Stat(ctx, key); errors.Is(err, ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	return fmt.Errorf("image object remains after purge: %w", ErrExists)
}

// PurgeOwned removes the local object only if its durable owner matches.
func (d *Local) PurgeOwned(ctx context.Context, key, owner string) error {
	if owner == "" {
		return ErrOwnership
	}
	if err := d.validate(ctx, key); err != nil {
		return err
	}
	if err := d.root.Remove(localStagingName(key, owner)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return safeError("remove owned staging", err)
	}
	info, err := d.Stat(ctx, key)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.OwnerID != owner {
		return ErrOwnership
	}
	return d.DeleteCurrent(ctx, key)
}

func localStagingName(key, owner string) string {
	digest := sha256.Sum256([]byte(key + "\x00" + owner))
	return ".imgnest-tmp-" + hex.EncodeToString(digest[:])
}

func (d *Local) openFile(ctx context.Context, key string) (*os.File, ObjectInfo, error) {
	if err := d.validate(ctx, key); err != nil {
		return nil, ObjectInfo{}, err
	}
	file, err := d.root.Open(key)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ObjectInfo{}, ErrNotFound
		}
		return nil, ObjectInfo{}, safeError("open object", err)
	}
	info, err := readLocalHeader(file)
	if err != nil {
		_ = file.Close()
		return nil, ObjectInfo{}, err
	}
	return file, info, nil
}
func readLocalHeader(file *os.File) (ObjectInfo, error) {
	var prefix [12]byte
	if _, err := io.ReadFull(file, prefix[:]); err != nil || string(prefix[:8]) != localMagic {
		return ObjectInfo{}, ErrOwnership
	}
	length := binary.BigEndian.Uint32(prefix[8:])
	if length == 0 || length > 16384 {
		return ObjectInfo{}, ErrOwnership
	}
	header := make([]byte, length)
	if _, err := io.ReadFull(file, header); err != nil {
		return ObjectInfo{}, ErrOwnership
	}
	var info ObjectInfo
	if err := json.Unmarshal(header, &info); err != nil || info.OwnerID == "" || info.Size < 0 {
		return ObjectInfo{}, ErrOwnership
	}
	stat, err := file.Stat()
	if err != nil {
		return ObjectInfo{}, safeError("stat object content", err)
	}
	if !stat.Mode().IsRegular() || stat.Size() != 12+int64(length)+info.Size {
		return ObjectInfo{}, ErrOwnership
	}
	return info, nil
}
func (d *Local) validate(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateKey(key); err != nil {
		return err
	}
	parts := strings.Split(key, "/")
	for i := range parts {
		info, err := d.root.Lstat(strings.Join(parts[:i+1], "/"))
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			return safeError("inspect object path", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink object path is not allowed")
		}
	}
	return nil
}
func validateKey(key string) error {
	if !utf8.ValidString(key) || key == "" || len(key) > 1024 || strings.ContainsAny(key, "\\:") {
		return fmt.Errorf("invalid object key")
	}
	for _, r := range key {
		if unicode.IsControl(r) {
			return fmt.Errorf("invalid object key")
		}
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" || part == "." || part == ".." || strings.HasPrefix(part, ".imgnest-") {
			return fmt.Errorf("invalid object key")
		}
	}
	return nil
}
func validateOptions(opts PutOptions) error {
	if opts.OwnerID == "" || len(opts.OwnerID) > 255 || len(opts.MIME) > 1024 || len(opts.CacheControl) > 1024 || strings.ContainsAny(opts.OwnerID+opts.MIME+opts.CacheControl, "\r\n\x00") {
		return fmt.Errorf("invalid object options")
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

type localReader struct {
	contextReader
	file *os.File
}

func (r *localReader) Close() error { return r.file.Close() }

type redactedError struct{ cause error }

func (e redactedError) Error() string { return "storage operation failed" }
func (e redactedError) Unwrap() error { return e.cause }
func safeError(operation string, err error) error {
	return fmt.Errorf("%s: %w", operation, redactedError{cause: err})
}
