package model

import (
	"errors"
	"io"
	"time"
)

// ErrBackupMismatch reports a backup that cannot be restored here: another
// database driver, another schema version, or a file that is not an ImgNest
// backup.
var ErrBackupMismatch = errors.New("backup does not match this installation")

// ErrBackupBusy reports a backup requested while another one is running.
var ErrBackupBusy = errors.New("backup already running")

// BackupStats describes the database snapshot inside a backup.
type BackupStats struct {
	Driver        string           `json:"driver"`
	SchemaVersion int64            `json:"schema_version"`
	Tables        map[string]int64 `json:"tables"`
}

// BackupFile is one archive in the backup directory.
type BackupFile struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
	// Auto marks archives the schedule wrote; only those are pruned.
	Auto bool `json:"auto"`
}

// BackupWriter receives a new archive; Commit publishes it under its name and
// Abort discards it, so a failed backup never leaves a partial file behind.
type BackupWriter interface {
	io.Writer
	Commit() error
	Abort() error
}

// BackupArchive is an open archive in the backup directory.
type BackupArchive interface {
	io.ReaderAt
	io.Reader
	io.Closer
}
