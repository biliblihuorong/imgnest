package service

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/biliblihuorong/imgnest/internal/logx"
	"github.com/biliblihuorong/imgnest/internal/model"
)

var (
	// ErrBackupBusy rejects a backup requested while another one runs.
	ErrBackupBusy = model.ErrBackupBusy
	// ErrBackupMismatch rejects restoring an archive made by another driver
	// or schema version, or a file that is not an ImgNest backup.
	ErrBackupMismatch = model.ErrBackupMismatch
)

// backupFormat versions the archive layout so a later release can still
// recognize and refuse or convert older archives.
const backupFormat = 1

// Archive entries. The configuration entries hold deployment secrets (DSN,
// master key), which a restore needs to read sealed storage credentials.
const (
	backupManifestEntry = "manifest.json"
	backupDatabaseEntry = "database.sql"
	backupConfigDir     = "config/"
)

// Schedule bounds.
const (
	maxBackupIntervalHours = 24 * 30
	maxBackupKeep          = 365
	backupScheduleKey      = "backup_schedule"
)

var backupName = regexp.MustCompile(`^imgnest-[0-9]{8}-[0-9]{6}(-auto)?\.zip$`)

// BackupDumper writes and replays a database snapshot.
type BackupDumper interface {
	Dump(context.Context, io.Writer) (model.BackupStats, error)
	Restore(ctx context.Context, driver string, schemaVersion int64, dump io.Reader) error
}

// BackupWriter receives a new archive from a BackupStore.
type BackupWriter = model.BackupWriter

// BackupArchive is an open archive from a BackupStore.
type BackupArchive = model.BackupArchive

// BackupStore keeps archives in the backup directory.
type BackupStore interface {
	Create(ctx context.Context, name string) (BackupWriter, error)
	Open(ctx context.Context, name string) (BackupArchive, model.BackupFile, error)
	List(ctx context.Context) ([]model.BackupFile, error)
	Remove(ctx context.Context, name string) error
	// Location names the directory for the console, such as "data/backup".
	Location() string
}

// BackupSettingsStore persists the schedule next to the site settings.
type BackupSettingsStore interface {
	ReadSettings(context.Context) (map[string]json.RawMessage, error)
	UpdateSettings(context.Context, map[string]json.RawMessage) error
}

// BackupConfigFile is one configuration file copied into an archive.
type BackupConfigFile struct {
	Name    string
	Content []byte
}

// BackupDependencies wires the backup service.
type BackupDependencies struct {
	Dumper   BackupDumper
	Store    BackupStore
	Settings BackupSettingsStore
	// Config returns the deployment configuration to include, read at backup
	// time so a changed file is captured.
	Config func(context.Context) ([]BackupConfigFile, error)
	Now    func() time.Time
	Logger *slog.Logger
}

// BackupSchedule is the automatic backup setting.
type BackupSchedule struct {
	Enabled       bool `json:"enabled"`
	IntervalHours int  `json:"interval_hours"`
	Keep          int  `json:"keep"`
}

// BackupManifest describes an archive's contents.
type BackupManifest struct {
	Format    int       `json:"format"`
	CreatedAt time.Time `json:"created_at"`
	model.BackupStats
	ConfigFiles []string `json:"config_files"`
}

// BackupOverview is the console's backup page.
type BackupOverview struct {
	Items    []model.BackupFile `json:"items"`
	Schedule BackupSchedule     `json:"schedule"`
	Running  bool               `json:"running"`
	Location string             `json:"location"`
}

// BackupService creates, lists and prunes database and configuration
// backups in one directory.
type BackupService struct {
	deps BackupDependencies
	// mu admits one backup at a time; running mirrors it for the console.
	mu      sync.Mutex
	running bool
}

// NewBackupService validates the dependencies.
func NewBackupService(ctx context.Context, deps BackupDependencies) (*BackupService, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("create backup service: %w", err)
	}
	if deps.Dumper == nil || deps.Store == nil || deps.Settings == nil {
		return nil, fmt.Errorf("create backup service: %w", ErrInvalidInput)
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Config == nil {
		deps.Config = func(context.Context) ([]BackupConfigFile, error) { return nil, nil }
	}
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.DiscardHandler)
	}
	return &BackupService{deps: deps}, nil
}

// Overview lists the archives, newest first, with the schedule.
func (s *BackupService) Overview(ctx context.Context) (BackupOverview, error) {
	items, err := s.list(ctx)
	if err != nil {
		return BackupOverview{}, err
	}
	schedule, err := s.schedule(ctx)
	if err != nil {
		return BackupOverview{}, err
	}
	s.mu.Lock()
	running := s.running
	s.mu.Unlock()
	return BackupOverview{Items: items, Schedule: schedule, Running: running, Location: s.deps.Store.Location()}, nil
}

// Create writes a new archive now.
func (s *BackupService) Create(ctx context.Context) (model.BackupFile, error) {
	return s.create(ctx, false)
}

func (s *BackupService) create(ctx context.Context, auto bool) (model.BackupFile, error) {
	if err := ctx.Err(); err != nil {
		return model.BackupFile{}, fmt.Errorf("create backup: %w", err)
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return model.BackupFile{}, ErrBackupBusy
	}
	s.running = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	now := s.deps.Now().UTC()
	name := "imgnest-" + now.Format("20060102-150405")
	if auto {
		name += "-auto"
	}
	name += ".zip"
	writer, err := s.deps.Store.Create(ctx, name)
	if err != nil {
		return model.BackupFile{}, fmt.Errorf("create backup: %w", err)
	}
	if err := s.writeArchive(ctx, writer, now); err != nil {
		return model.BackupFile{}, errors.Join(fmt.Errorf("create backup: %w", err), writer.Abort())
	}
	if err := writer.Commit(); err != nil {
		return model.BackupFile{}, fmt.Errorf("create backup: %w", err)
	}
	// Only the metadata is needed; the archive handle is released here.
	archive, file, err := s.open(ctx, name)
	if err != nil {
		return model.BackupFile{}, err
	}
	if err := archive.Close(); err != nil {
		return model.BackupFile{}, fmt.Errorf("close backup: %w", err)
	}
	return file, nil
}

func (s *BackupService) writeArchive(ctx context.Context, out io.Writer, now time.Time) error {
	archive := zip.NewWriter(out)
	entry := func(name string) (io.Writer, error) {
		return archive.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: now})
	}
	database, err := entry(backupDatabaseEntry)
	if err != nil {
		return err
	}
	stats, err := s.deps.Dumper.Dump(ctx, database)
	if err != nil {
		return err
	}
	files, err := s.deps.Config(ctx)
	if err != nil {
		return err
	}
	manifest := BackupManifest{Format: backupFormat, CreatedAt: now, BackupStats: stats, ConfigFiles: []string{}}
	for _, file := range files {
		w, err := entry(backupConfigDir + file.Name)
		if err != nil {
			return err
		}
		if _, err := w.Write(file.Content); err != nil {
			return err
		}
		manifest.ConfigFiles = append(manifest.ConfigFiles, backupConfigDir+file.Name)
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	w, err := entry(backupManifestEntry)
	if err != nil {
		return err
	}
	if _, err := w.Write(encoded); err != nil {
		return err
	}
	return archive.Close()
}

// Open returns an archive for download; the caller closes it.
func (s *BackupService) Open(ctx context.Context, name string) (BackupArchive, model.BackupFile, error) {
	return s.open(ctx, name)
}

func (s *BackupService) open(ctx context.Context, name string) (BackupArchive, model.BackupFile, error) {
	if !backupName.MatchString(name) {
		return nil, model.BackupFile{}, ErrNotFound
	}
	archive, file, err := s.deps.Store.Open(ctx, name)
	if err != nil {
		return nil, model.BackupFile{}, fmt.Errorf("open backup: %w", err)
	}
	file.Auto = isAutoBackup(name)
	return archive, file, nil
}

// Delete removes one archive.
func (s *BackupService) Delete(ctx context.Context, name string) error {
	if !backupName.MatchString(name) {
		return ErrNotFound
	}
	if err := s.deps.Store.Remove(ctx, name); err != nil {
		return fmt.Errorf("delete backup: %w", err)
	}
	return nil
}

// PutSchedule validates and stores the automatic backup setting.
func (s *BackupService) PutSchedule(ctx context.Context, schedule BackupSchedule) (BackupSchedule, error) {
	if err := ctx.Err(); err != nil {
		return BackupSchedule{}, fmt.Errorf("write backup schedule: %w", err)
	}
	if !validBackupSchedule(schedule) {
		return BackupSchedule{}, ErrInvalidInput
	}
	encoded, err := json.Marshal(schedule)
	if err != nil {
		return BackupSchedule{}, ErrInvalidInput
	}
	if err := s.deps.Settings.UpdateSettings(ctx, map[string]json.RawMessage{backupScheduleKey: encoded}); err != nil {
		return BackupSchedule{}, fmt.Errorf("write backup schedule: %w", err)
	}
	return schedule, nil
}

// RunScheduled writes an automatic backup when the schedule is due and then
// keeps only the newest automatic archives; manual ones are never pruned.
func (s *BackupService) RunScheduled(ctx context.Context) error {
	schedule, err := s.schedule(ctx)
	if err != nil || !schedule.Enabled {
		return err
	}
	items, err := s.list(ctx)
	if err != nil {
		return err
	}
	var latest time.Time
	for _, item := range items {
		if item.Auto && item.CreatedAt.After(latest) {
			latest = item.CreatedAt
		}
	}
	if !latest.IsZero() && s.deps.Now().Sub(latest) < time.Duration(schedule.IntervalHours)*time.Hour {
		return nil
	}
	if _, err := s.create(ctx, true); err != nil {
		if errors.Is(err, ErrBackupBusy) {
			return nil
		}
		return err
	}
	items, err = s.list(ctx)
	if err != nil {
		return err
	}
	kept := 0
	for _, item := range items {
		if !item.Auto {
			continue
		}
		kept++
		if kept > schedule.Keep {
			if err := s.deps.Store.Remove(ctx, item.Name); err != nil {
				return fmt.Errorf("prune backup: %w", err)
			}
		}
	}
	return nil
}

// RunSchedule checks the schedule until ctx ends.
func (s *BackupService) RunSchedule(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.RunScheduled(ctx); err != nil && ctx.Err() == nil {
				s.deps.Logger.ErrorContext(ctx, "scheduled backup failed", "code", 50001, "error", logx.Redact(err.Error()))
			}
		}
	}
}

// Restore replaces the database with the archive's snapshot. The archive must
// come from the same driver and schema version; configuration files are left
// for the operator to copy back.
func (s *BackupService) Restore(ctx context.Context, archive io.ReaderAt, size int64) (BackupManifest, error) {
	reader, err := zip.NewReader(archive, size)
	if err != nil {
		return BackupManifest{}, ErrBackupMismatch
	}
	var manifestFile, databaseFile *zip.File
	for _, file := range reader.File {
		switch file.Name {
		case backupManifestEntry:
			manifestFile = file
		case backupDatabaseEntry:
			databaseFile = file
		}
	}
	if manifestFile == nil || databaseFile == nil {
		return BackupManifest{}, ErrBackupMismatch
	}
	manifest, err := readBackupManifest(manifestFile)
	if err != nil {
		return BackupManifest{}, err
	}
	dump, err := databaseFile.Open()
	if err != nil {
		return BackupManifest{}, ErrBackupMismatch
	}
	defer func() { _ = dump.Close() }()
	if err := s.deps.Dumper.Restore(ctx, manifest.Driver, manifest.SchemaVersion, dump); err != nil {
		return manifest, fmt.Errorf("restore backup: %w", err)
	}
	return manifest, nil
}

func readBackupManifest(file *zip.File) (BackupManifest, error) {
	in, err := file.Open()
	if err != nil {
		return BackupManifest{}, ErrBackupMismatch
	}
	defer func() { _ = in.Close() }()
	var manifest BackupManifest
	if err := json.NewDecoder(io.LimitReader(in, 1<<20)).Decode(&manifest); err != nil || manifest.Format != backupFormat {
		return BackupManifest{}, ErrBackupMismatch
	}
	return manifest, nil
}

func (s *BackupService) list(ctx context.Context) ([]model.BackupFile, error) {
	files, err := s.deps.Store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	items := make([]model.BackupFile, 0, len(files))
	for _, file := range files {
		if !backupName.MatchString(file.Name) {
			continue
		}
		file.Auto = isAutoBackup(file.Name)
		items = append(items, file)
	}
	// Names embed the UTC creation time, so name order is age order.
	sort.Slice(items, func(i, j int) bool { return items[i].Name > items[j].Name })
	return items, nil
}

func (s *BackupService) schedule(ctx context.Context) (BackupSchedule, error) {
	schedule := BackupSchedule{IntervalHours: 24, Keep: 7}
	values, err := s.deps.Settings.ReadSettings(ctx)
	if err != nil {
		return BackupSchedule{}, fmt.Errorf("read backup schedule: %w", err)
	}
	if value, ok := values[backupScheduleKey]; ok {
		var stored BackupSchedule
		// A corrupt value keeps automatic backups off instead of failing the page.
		if json.Unmarshal(value, &stored) == nil && validBackupSchedule(stored) {
			schedule = stored
		}
	}
	return schedule, nil
}

func validBackupSchedule(schedule BackupSchedule) bool {
	return schedule.IntervalHours >= 1 && schedule.IntervalHours <= maxBackupIntervalHours &&
		schedule.Keep >= 1 && schedule.Keep <= maxBackupKeep
}

func isAutoBackup(name string) bool {
	return strings.HasSuffix(name, "-auto.zip")
}
