// Package backupdir keeps backup archives in one local directory.
package backupdir

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// Archives hold password hashes and deployment secrets, so only the server's
// own user may read them.
const (
	dirMode  = 0o700
	fileMode = 0o600
)

// Dir is a backup directory.
type Dir struct{ path string }

// New prepares the directory, creating it when missing.
func New(ctx context.Context, path string) (*Dir, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("open backup directory: %w", err)
	}
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("open backup directory: %w", model.ErrInvalidInput)
	}
	if err := os.MkdirAll(path, dirMode); err != nil {
		return nil, fmt.Errorf("open backup directory: %w", storageError(err))
	}
	return &Dir{path: path}, nil
}

// Location returns the directory as configured.
func (d *Dir) Location() string { return d.path }

// Create starts a new archive in a hidden temporary file; Commit renames it
// into place, so a crash never leaves a half-written archive under its name.
func (d *Dir) Create(ctx context.Context, name string) (model.BackupWriter, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	target, err := d.resolve(name)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(target); err == nil {
		return nil, model.ErrBackupBusy
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, storageError(err)
	}
	file, err := os.CreateTemp(d.path, ".partial-*")
	if err != nil {
		return nil, storageError(err)
	}
	if err := file.Chmod(fileMode); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, storageError(err)
	}
	return &writer{file: file, target: target}, nil
}

type writer struct {
	file   *os.File
	target string
}

func (w *writer) Write(p []byte) (int, error) {
	n, err := w.file.Write(p)
	if err != nil {
		return n, storageError(err)
	}
	return n, nil
}

func (w *writer) Commit() error {
	if err := w.file.Sync(); err != nil {
		return errors.Join(storageError(err), w.Abort())
	}
	if err := w.file.Close(); err != nil {
		_ = os.Remove(w.file.Name())
		return storageError(err)
	}
	if err := os.Rename(w.file.Name(), w.target); err != nil {
		_ = os.Remove(w.file.Name())
		return storageError(err)
	}
	return nil
}

func (w *writer) Abort() error {
	_ = w.file.Close()
	if err := os.Remove(w.file.Name()); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return storageError(err)
	}
	return nil
}

// Open opens one archive for reading.
func (d *Dir) Open(ctx context.Context, name string) (model.BackupArchive, model.BackupFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, model.BackupFile{}, err
	}
	target, err := d.resolve(name)
	if err != nil {
		return nil, model.BackupFile{}, err
	}
	file, err := os.Open(target) //nolint:gosec // resolve admits only a plain name inside the directory
	if errors.Is(err, fs.ErrNotExist) {
		return nil, model.BackupFile{}, model.ErrNotFound
	}
	if err != nil {
		return nil, model.BackupFile{}, storageError(err)
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, model.BackupFile{}, model.ErrNotFound
	}
	return file, model.BackupFile{Name: name, Size: info.Size(), CreatedAt: info.ModTime().UTC()}, nil
}

// List returns the regular files in the directory; temporary files are skipped.
func (d *Dir) List(ctx context.Context) ([]model.BackupFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(d.path)
	if err != nil {
		return nil, storageError(err)
	}
	files := make([]model.BackupFile, 0, len(entries))
	for _, entry := range entries {
		if !entry.Type().IsRegular() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, model.BackupFile{Name: entry.Name(), Size: info.Size(), CreatedAt: info.ModTime().UTC()})
	}
	return files, nil
}

// Remove deletes one archive.
func (d *Dir) Remove(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target, err := d.resolve(name)
	if err != nil {
		return err
	}
	if err := os.Remove(target); errors.Is(err, fs.ErrNotExist) {
		return model.ErrNotFound
	} else if err != nil {
		return storageError(err)
	}
	return nil
}

// resolve accepts only a plain file name inside the directory.
func (d *Dir) resolve(name string) (string, error) {
	if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\`) {
		return "", model.ErrNotFound
	}
	return filepath.Join(d.path, name), nil
}

// storageError keeps the cause for reason hints (disk full, read-only)
// while reporting a generic storage failure.
func storageError(err error) error {
	return fmt.Errorf("%w: %w", model.ErrStorage, err)
}
