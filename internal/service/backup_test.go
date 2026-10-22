package service_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/backupdir"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
)

type backupDumperFake struct {
	restoredDriver  string
	restoredVersion int64
	restoredSQL     string
	fail            error
}

func (f *backupDumperFake) Dump(_ context.Context, w io.Writer) (model.BackupStats, error) {
	if f.fail != nil {
		return model.BackupStats{}, f.fail
	}
	_, err := io.WriteString(w, "INSERT INTO \"settings\" (\"key\") VALUES ('x');\n")
	return model.BackupStats{Driver: "sqlite", SchemaVersion: 13, Tables: map[string]int64{"settings": 1}}, err
}

func (f *backupDumperFake) Restore(_ context.Context, driver string, version int64, dump io.Reader) error {
	content, err := io.ReadAll(dump)
	f.restoredDriver, f.restoredVersion, f.restoredSQL = driver, version, string(content)
	return err
}

type backupSettingsFake struct{ values map[string]json.RawMessage }

func (f *backupSettingsFake) ReadSettings(context.Context) (map[string]json.RawMessage, error) {
	return f.values, nil
}

func (f *backupSettingsFake) UpdateSettings(_ context.Context, values map[string]json.RawMessage) error {
	for key, value := range values {
		f.values[key] = value
	}
	return nil
}

type backupFixture struct {
	service  *service.BackupService
	dumper   *backupDumperFake
	settings *backupSettingsFake
	dir      string
	now      *time.Time
}

func newBackupFixture(t *testing.T) backupFixture {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "backup")
	store, err := backupdir.New(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)
	fixture := backupFixture{dumper: &backupDumperFake{}, settings: &backupSettingsFake{values: map[string]json.RawMessage{}}, dir: dir, now: &now}
	fixture.service, err = service.NewBackupService(t.Context(), service.BackupDependencies{
		Dumper: fixture.dumper, Store: store, Settings: fixture.settings,
		Config: func(context.Context) ([]service.BackupConfigFile, error) {
			return []service.BackupConfigFile{{Name: "imgnest.yaml", Content: []byte("server:\n  addr: :8080\n")}}, nil
		},
		Now: func() time.Time { return *fixture.now },
	})
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

// countingStore records archives opened through the real store and whether
// each was closed.
type countingStore struct {
	service.BackupStore
	open []*trackedArchive
}

type trackedArchive struct {
	model.BackupArchive
	closed bool
}

func (a *trackedArchive) Close() error {
	a.closed = true
	return a.BackupArchive.Close()
}

func (s *countingStore) Open(ctx context.Context, name string) (model.BackupArchive, model.BackupFile, error) {
	archive, file, err := s.BackupStore.Open(ctx, name)
	if err != nil {
		return nil, file, err
	}
	tracked := &trackedArchive{BackupArchive: archive}
	s.open = append(s.open, tracked)
	return tracked, file, nil
}

func TestBackupCreateClosesTheArchiveItReadsMetadataFrom(t *testing.T) {
	store, err := backupdir.New(t.Context(), filepath.Join(t.TempDir(), "backup"))
	if err != nil {
		t.Fatal(err)
	}
	counting := &countingStore{BackupStore: store}
	backups, err := service.NewBackupService(t.Context(), service.BackupDependencies{
		Dumper: &backupDumperFake{}, Store: counting, Settings: &backupSettingsFake{values: map[string]json.RawMessage{}},
		Config: func(context.Context) ([]service.BackupConfigFile, error) { return nil, nil },
		Now:    func() time.Time { return time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	file, err := backups.Create(t.Context())
	if err != nil || file.Size == 0 {
		t.Fatalf("create: %+v, %v", file, err)
	}
	if len(counting.open) != 1 || !counting.open[0].closed {
		t.Fatalf("archives opened=%d, first closed=%v; want the metadata handle closed", len(counting.open), len(counting.open) > 0 && counting.open[0].closed)
	}
}

func TestBackupCreateWritesDatabaseConfigAndManifest(t *testing.T) {
	fixture := newBackupFixture(t)
	file, err := fixture.service.Create(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if file.Name != "imgnest-20261016-080000.zip" || file.Auto || file.Size == 0 {
		t.Fatalf("file = %+v", file)
	}
	info, err := os.Stat(filepath.Join(fixture.dir, file.Name))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("archive mode = %v, want 0600", info.Mode().Perm())
	}
	archive, err := zip.OpenReader(filepath.Join(fixture.dir, file.Name))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = archive.Close() }()
	entries := map[string]string{}
	for _, entry := range archive.File {
		in, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, _ := io.ReadAll(in)
		_ = in.Close()
		entries[entry.Name] = string(content)
	}
	if !strings.HasPrefix(entries["database.sql"], "INSERT INTO") || !strings.Contains(entries["config/imgnest.yaml"], "addr") {
		t.Fatalf("entries = %v", entries)
	}
	var manifest service.BackupManifest
	if err := json.Unmarshal([]byte(entries["manifest.json"]), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Format != 1 || manifest.Driver != "sqlite" || manifest.SchemaVersion != 13 || manifest.ConfigFiles[0] != "config/imgnest.yaml" {
		t.Fatalf("manifest = %+v", manifest)
	}

	// A second backup in the same second does not overwrite the first.
	if _, err := fixture.service.Create(t.Context()); !errors.Is(err, service.ErrBackupBusy) {
		t.Fatalf("same-second backup err = %v, want ErrBackupBusy", err)
	}
}

func TestBackupCreateFailureLeavesNoFile(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.dumper.fail = errors.New("database is gone")
	if _, err := fixture.service.Create(t.Context()); err == nil {
		t.Fatal("Create succeeded with a failing dump")
	}
	entries, err := os.ReadDir(fixture.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed backup left %d files", len(entries))
	}
}

func TestBackupOpenAndDeleteRejectForeignNames(t *testing.T) {
	fixture := newBackupFixture(t)
	if err := os.WriteFile(filepath.Join(fixture.dir, "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../imgnest.db", "notes.txt", ".partial-1", "imgnest-20261016-080000.zip/.."} {
		if _, _, err := fixture.service.Open(t.Context(), name); !errors.Is(err, service.ErrNotFound) {
			t.Errorf("Open(%q) err = %v", name, err)
		}
		if err := fixture.service.Delete(t.Context(), name); !errors.Is(err, service.ErrNotFound) {
			t.Errorf("Delete(%q) err = %v", name, err)
		}
	}
	overview, err := fixture.service.Overview(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Items) != 0 || overview.Location != fixture.dir || overview.Schedule != (service.BackupSchedule{IntervalHours: 24, Keep: 7}) {
		t.Fatalf("overview = %+v", overview)
	}
}

func TestBackupScheduleRunsWhenDueAndPrunesOnlyAutomaticArchives(t *testing.T) {
	fixture := newBackupFixture(t)
	// The schedule compares the clock with archive file times, which the
	// filesystem stamps from the real clock; a fixed past date would make
	// every archive look newer than "now" and nothing would ever be due.
	*fixture.now = time.Now().UTC().Truncate(time.Hour)
	ctx := t.Context()
	// Disabled by default.
	if err := fixture.service.RunScheduled(ctx); err != nil {
		t.Fatal(err)
	}
	manual, err := fixture.service.Create(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []service.BackupSchedule{{Enabled: true, IntervalHours: 0, Keep: 1}, {Enabled: true, IntervalHours: 1, Keep: 0}, {Enabled: true, IntervalHours: 721, Keep: 1}} {
		if _, err := fixture.service.PutSchedule(ctx, bad); !errors.Is(err, service.ErrInvalidInput) {
			t.Fatalf("PutSchedule(%+v) err = %v", bad, err)
		}
	}
	if _, err := fixture.service.PutSchedule(ctx, service.BackupSchedule{Enabled: true, IntervalHours: 6, Keep: 2}); err != nil {
		t.Fatal(err)
	}
	for range 4 {
		*fixture.now = fixture.now.Add(time.Hour)
		if err := fixture.service.RunScheduled(ctx); err != nil {
			t.Fatal(err)
		}
		// Not yet due: nothing more is written within the interval.
		*fixture.now = fixture.now.Add(time.Hour)
		if err := fixture.service.RunScheduled(ctx); err != nil {
			t.Fatal(err)
		}
		*fixture.now = fixture.now.Add(5 * time.Hour)
	}
	overview, err := fixture.service.Overview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var auto, kept []string
	for _, item := range overview.Items {
		if item.Auto {
			auto = append(auto, item.Name)
		} else {
			kept = append(kept, item.Name)
		}
	}
	if len(auto) != 2 || len(kept) != 1 || kept[0] != manual.Name {
		t.Fatalf("items = %+v", overview.Items)
	}
	if auto[0] <= auto[1] {
		t.Fatalf("items are not newest first: %v", auto)
	}
}

func TestBackupRestoreReplaysTheArchivedDatabase(t *testing.T) {
	fixture := newBackupFixture(t)
	file, err := fixture.service.Create(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	archive, info, err := fixture.service.Open(t.Context(), file.Name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = archive.Close() }()
	manifest, err := fixture.service.Restore(t.Context(), archive, info.Size)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 13 || fixture.dumper.restoredDriver != "sqlite" || fixture.dumper.restoredVersion != 13 || !strings.HasPrefix(fixture.dumper.restoredSQL, "INSERT INTO") {
		t.Fatalf("manifest = %+v, restore = %+v", manifest, fixture.dumper)
	}
	junk := []byte("not a zip archive")
	if _, err := fixture.service.Restore(t.Context(), bytes.NewReader(junk), int64(len(junk))); !errors.Is(err, service.ErrBackupMismatch) {
		t.Fatalf("junk restore err = %v", err)
	}
}
