package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/migrate"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/secret"
	"github.com/biliblihuorong/imgnest/internal/storage"
	"gorm.io/gorm"
)

// Only image decoding is stubbed: policy selection, reservations, commits,
// reference counts and storage updates use the migrated database repository;
// object writes and the admin connectivity probe use real local drivers.
type storageRaceFixture struct {
	db         *gorm.DB
	upload     *ImageService
	admin      *AdminService
	stores     *repo.StorageRepository
	images     *repo.ImageRepository
	backend    model.Storage
	subject    TokenSubject
	old, next  *storage.Local
	nextConfig json.RawMessage
}

func forStorageRaceDatabases(t *testing.T, run func(*testing.T, string)) {
	t.Helper()
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) { run(t, driver) })
	}
}

func newStorageRaceFixture(t *testing.T, driver string, guest bool) storageRaceFixture {
	t.Helper()
	cfg := config.Database{Driver: driver, DSN: filepath.Join(t.TempDir(), "race.db"), MaxOpen: 5, MaxIdle: 5}
	if driver == "postgres" {
		cfg.DSN = os.Getenv("IMGNEST_TEST_POSTGRES_DSN")
		if cfg.DSN == "" {
			t.Skip("IMGNEST_TEST_POSTGRES_DSN is not set")
		}
		admin, err := repo.Open(t.Context(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		adminDB, err := admin.DB()
		if err != nil {
			t.Fatal(err)
		}
		random, err := operationID()
		if err != nil {
			t.Fatal(err)
		}
		schema := "storage_race_" + random
		if _, err := adminDB.ExecContext(t.Context(), "CREATE SCHEMA "+schema); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := adminDB.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
				t.Error(err)
			}
			if err := adminDB.Close(); err != nil {
				t.Error(err)
			}
		})
		if strings.Contains(cfg.DSN, "://") {
			parsed, err := url.Parse(cfg.DSN)
			if err != nil {
				t.Fatal("invalid test database URL")
			}
			query := parsed.Query()
			query.Set("search_path", schema)
			parsed.RawQuery = query.Encode()
			cfg.DSN = parsed.String()
		} else {
			cfg.DSN += " search_path=" + schema
		}
	}
	db, err := repo.Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := migrate.Up(t.Context(), sqlDB, driver); err != nil {
		t.Fatal(err)
	}
	users, err := repo.NewUserRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := repo.NewSettingsRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	groupID, err := settings.DefaultGroupID(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	user, err := users.CreateUser(t.Context(), model.User{Username: "storage-race", Email: "storage-race@example.com", PasswordHash: "verified", GroupID: groupID, Role: model.UserRoleUser, Status: model.UserStatusEnabled})
	if err != nil {
		t.Fatal(err)
	}
	stores, err := repo.NewStorageRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	oldRoot, nextRoot := filepath.Join(t.TempDir(), "old"), filepath.Join(t.TempDir(), "next")
	oldConfig, _ := json.Marshal(map[string]string{"root": oldRoot})
	nextConfig, _ := json.Marshal(map[string]string{"root": nextRoot})
	backend, err := stores.Create(t.Context(), model.Storage{Name: "disk", Driver: "local", Config: oldConfig, BaseURL: "http://images.test/files", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	old, err := storage.NewLocal(t.Context(), oldRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = old.Close() })
	next, err := storage.NewLocal(t.Context(), nextRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = next.Close() })
	policies, err := repo.NewPolicyRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	upload, _, source, _, subject := uploadFixture(t, "png")
	policy := source.policy
	policy.ID, policy.StorageID = 0, backend.ID
	policy.Name = "race policy"
	if guest {
		group := model.Group{Name: "Guests", IsGuest: true, AllowedExts: []string{"png"}}
		if err := db.Create(&group).Error; err != nil {
			t.Fatal(err)
		}
		groupID = group.ID
	}
	if _, err := policies.CreateAndBind(t.Context(), policy, groupID, true); err != nil {
		t.Fatal(err)
	}
	images, err := repo.NewImageRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	refs, err := repo.NewAdminRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	upload.deps.Images, upload.deps.Policies, upload.deps.Storages, upload.deps.Users = images, policies, stores, users
	upload.deps.Drivers = uploadDriverProvider{driver: old}
	subject.userID, subject.passwordHash = user.ID, user.PasswordHash
	admin, _, _, _, _, _, _ := adminServiceFixture(t, AdminDependencies{Storages: stores, References: refs, Drivers: uploadDriverProvider{driver: next}})
	return storageRaceFixture{db: db, upload: upload, admin: admin, stores: stores, images: images, backend: backend, subject: subject, old: old, next: next, nextConfig: nextConfig}
}

// The barrier pauses immediately before the real object probe (admin), or
// after an upload selected its old driver but before it reserves its path.
type storageDriverBarrier struct {
	driver  storage.Driver
	reached chan struct{}
	resume  chan struct{}
	once    sync.Once
}

func (p *storageDriverBarrier) DriverFor(ctx context.Context, _ model.Storage) (storage.Driver, error) {
	p.once.Do(func() { close(p.reached) })
	select {
	case <-p.resume:
		return p.driver, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func waitStorageBarrier(ctx context.Context, t *testing.T, reached <-chan struct{}) {
	t.Helper()
	select {
	case <-reached:
	case <-ctx.Done():
		t.Fatal("storage race did not reach its barrier")
	}
}

func (f storageRaceFixture) publish(ctx context.Context, guest bool) (ImageView, error) {
	input := UploadInput{Data: []byte("source"), Filename: "first.png"}
	if guest {
		return f.upload.GuestUpload(ctx, input)
	}
	return f.upload.Upload(ctx, f.subject, input)
}

func assertStorageRaceRoot(t *testing.T, got, want json.RawMessage) {
	t.Helper()
	var actual, expected struct{ Root string }
	if err := json.Unmarshal(got, &actual); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &expected); err != nil {
		t.Fatal(err)
	}
	if actual.Root != expected.Root {
		t.Fatal("storage operation changed the expected object root")
	}
}

// A relocation between driver selection and reservation must abort the upload
// before either backend receives image bytes, for users and guests alike.
func TestStorageRelocationRejectsStaleUploadBeforeObjectWrite(t *testing.T) {
	forStorageRaceDatabases(t, func(t *testing.T, driver string) {
		for _, guest := range []bool{false, true} {
			name := "account"
			if guest {
				name = "guest"
			}
			t.Run(name, func(t *testing.T) {
				f := newStorageRaceFixture(t, driver, guest)
				ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
				defer cancel()
				barrier := &storageDriverBarrier{driver: f.old, reached: make(chan struct{}), resume: make(chan struct{})}
				f.upload.deps.Drivers = barrier
				done := make(chan error, 1)
				go func() { _, err := f.publish(ctx, guest); done <- err }()
				waitStorageBarrier(ctx, t, barrier.reached)
				if _, err := f.admin.PatchStorage(ctx, f.backend.ID, StoragePatch{Config: f.nextConfig}); err != nil {
					t.Fatal(err)
				}
				close(barrier.resume)
				select {
				case err := <-done:
					if !errors.Is(err, ErrImageBusy) {
						t.Fatalf("upload using pre-relocation driver error=%v, want ErrImageBusy", err)
					}
				case <-ctx.Done():
					t.Fatal("upload did not finish")
				}
				var rows int64
				if err := f.db.Model(&model.Image{}).Count(&rows).Error; err != nil {
					t.Fatal(err)
				}
				if rows != 0 {
					t.Fatalf("stale upload persisted %d image rows", rows)
				}
				for _, driver := range []*storage.Local{f.old, f.next} {
					if _, err := driver.Stat(ctx, "2026/10/first.png"); !errors.Is(err, storage.ErrNotFound) {
						t.Fatalf("stale upload wrote an object: %v", err)
					}
				}
			})
		}
	})
}

type storageReservationBarrier struct {
	*repo.ImageRepository
	reached chan struct{}
	resume  chan struct{}
}

func (r *storageReservationBarrier) ReserveUpload(ctx context.Context, req model.UploadReservation) (model.Image, error) {
	image, err := r.ImageRepository.ReserveUpload(ctx, req)
	if err != nil {
		return model.Image{}, err
	}
	close(r.reached)
	select {
	case <-r.resume:
		return image, nil
	case <-ctx.Done():
		return model.Image{}, ctx.Err()
	}
}

func TestStorageRelocationRechecksImagesAfterConnectivityProbe(t *testing.T) {
	forStorageRaceDatabases(t, func(t *testing.T, driver string) {
		for _, state := range []string{"pending", "active", "trash"} {
			t.Run(state, func(t *testing.T) {
				f := newStorageRaceFixture(t, driver, false)
				ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
				defer cancel()
				probe := &storageDriverBarrier{driver: f.next, reached: make(chan struct{}), resume: make(chan struct{})}
				f.admin.deps.Drivers = probe
				patched := make(chan error, 1)
				go func() {
					_, err := f.admin.PatchStorage(ctx, f.backend.ID, StoragePatch{Config: f.nextConfig})
					patched <- err
				}()
				waitStorageBarrier(ctx, t, probe.reached)
				var reservation *storageReservationBarrier
				var uploaded chan error
				if state == "pending" {
					reservation = &storageReservationBarrier{ImageRepository: f.images, reached: make(chan struct{}), resume: make(chan struct{})}
					f.upload.deps.Images = reservation
					uploaded = make(chan error, 1)
					go func() { _, err := f.publish(ctx, false); uploaded <- err }()
					waitStorageBarrier(ctx, t, reservation.reached)
				} else {
					view, err := f.publish(ctx, false)
					if err != nil {
						t.Fatal(err)
					}
					if state == "trash" {
						if err := f.upload.Trash(ctx, f.subject, view.Key); err != nil {
							t.Fatal(err)
						}
					}
				}
				close(probe.resume)
				select {
				case err := <-patched:
					if !errors.Is(err, ErrStillReferenced) {
						t.Errorf("relocation with %s image error=%v, want ErrStillReferenced", state, err)
					}
				case <-ctx.Done():
					t.Fatal("storage patch did not finish")
				}
				if reservation != nil {
					close(reservation.resume)
					select {
					case err := <-uploaded:
						if err != nil {
							t.Errorf("reserved upload failed after rejected move: %v", err)
						}
					case <-ctx.Done():
						t.Fatal("reserved upload did not finish")
					}
				}
				backend, err := f.stores.Find(ctx, f.backend.ID)
				if err != nil {
					t.Fatal(err)
				}
				assertStorageRaceRoot(t, backend.Config, f.backend.Config)
			})
		}
	})
}

// An older settings request must not restore the old location after another
// administrator successfully relocates an empty backend.
func TestStorageRelocationRejectsStaleAdministratorConfig(t *testing.T) {
	forStorageRaceDatabases(t, func(t *testing.T, driver string) {
		f := newStorageRaceFixture(t, driver, false)
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		probe := &storageDriverBarrier{driver: f.old, reached: make(chan struct{}), resume: make(chan struct{})}
		f.admin.deps.Drivers = probe
		patched := make(chan error, 1)
		go func() {
			_, err := f.admin.PatchStorage(ctx, f.backend.ID, StoragePatch{Config: f.backend.Config})
			patched <- err
		}()
		waitStorageBarrier(ctx, t, probe.reached)
		other, _, _, _, _, _, _ := adminServiceFixture(t, AdminDependencies{Storages: f.stores, References: f.admin.deps.References, Drivers: uploadDriverProvider{driver: f.next}})
		if _, err := other.PatchStorage(ctx, f.backend.ID, StoragePatch{Config: f.nextConfig}); err != nil {
			t.Fatal(err)
		}
		close(probe.resume)
		select {
		case err := <-patched:
			if !errors.Is(err, ErrImageBusy) {
				t.Fatalf("stale administrator error=%v, want ErrImageBusy", err)
			}
		case <-ctx.Done():
			t.Fatal("stale administrator did not finish")
		}
		backend, err := f.stores.Find(ctx, f.backend.ID)
		if err != nil {
			t.Fatal(err)
		}
		assertStorageRaceRoot(t, backend.Config, f.nextConfig)
	})
}

// This holds the real PostgreSQL update transaction after its empty-image
// check. Observe the upload's lock wait in PostgreSQL itself, then let the
// relocation commit: the old driver must fail its reservation afterward.
func TestStorageRelocationHoldsLockUntilUpdateCommit(t *testing.T) {
	f := newStorageRaceFixture(t, "postgres", false)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	driver := &storageDriverBarrier{driver: f.old, reached: make(chan struct{}), resume: make(chan struct{})}
	f.upload.deps.Drivers = driver
	uploaded := make(chan error, 1)
	go func() { _, err := f.publish(ctx, false); uploaded <- err }()
	waitStorageBarrier(ctx, t, driver.reached)
	updating, resume := make(chan int, 1), make(chan struct{})
	var release sync.Once
	defer release.Do(func() { close(resume) })
	const callback = "test:pause_storage_update_before_commit"
	if err := f.db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Table != "storages" {
			return
		}
		var pid int
		if err := tx.Session(&gorm.Session{NewDB: true}).Raw("SELECT pg_backend_pid()").Scan(&pid).Error; err != nil {
			_ = tx.AddError(err)
			return
		}
		updating <- pid
		select {
		case <-resume:
		case <-ctx.Done():
			_ = tx.AddError(ctx.Err())
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := f.db.Callback().Update().Remove(callback); err != nil {
			t.Error(err)
		}
	}()
	patched := make(chan error, 1)
	go func() {
		_, err := f.admin.PatchStorage(ctx, f.backend.ID, StoragePatch{Config: f.nextConfig})
		patched <- err
	}()
	var patchPID int
	select {
	case patchPID = <-updating:
	case <-ctx.Done():
		t.Fatal("relocation never reached its guarded update")
	}
	close(driver.resume)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked int64
		if err := f.db.WithContext(ctx).Raw("SELECT COUNT(*) FROM pg_stat_activity WHERE ? = ANY(pg_blocking_pids(pid))", patchPID).Scan(&blocked).Error; err != nil {
			t.Fatal(err)
		}
		if blocked > 0 {
			break
		}
		select {
		case err := <-uploaded:
			t.Fatalf("upload completed inside the relocation transaction instead of waiting for its storage lock: %v", err)
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("PostgreSQL did not observe the upload waiting for the storage lock")
		}
	}
	release.Do(func() { close(resume) })
	select {
	case err := <-patched:
		if err != nil {
			t.Fatalf("empty relocation failed: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("relocation did not commit")
	}
	select {
	case err := <-uploaded:
		if !errors.Is(err, ErrImageBusy) {
			t.Fatalf("stale upload after relocation error=%v, want ErrImageBusy", err)
		}
	case <-ctx.Done():
		t.Fatal("upload did not finish after relocation")
	}
}

func TestStorageReferencedBackendAllowsCredentialAndDisplayChanges(t *testing.T) {
	forStorageRaceDatabases(t, func(t *testing.T, driver string) {
		f := newStorageRaceFixture(t, driver, false)
		codec, err := secret.NewCodec(t.Context(), base64.StdEncoding.EncodeToString(make([]byte, 32)))
		if err != nil {
			t.Fatal(err)
		}
		before := json.RawMessage(`{"endpoint":"https://s3.test","region":"us-east-1","bucket":"photos","access_key_id":"old-test-key","secret_access_key":"old-test-secret"}`)
		sealed, err := codec.Seal(t.Context(), "s3", before)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.db.Model(&model.Storage{}).Where("id = ?", f.backend.ID).Updates(map[string]any{"driver": "s3", "config": string(sealed)}).Error; err != nil {
			t.Fatal(err)
		}
		f.admin.deps.Secrets, f.admin.deps.Drivers = codec, uploadDriverProvider{driver: f.old}
		view, err := f.publish(t.Context(), false)
		if err != nil {
			t.Fatal(err)
		}
		after := json.RawMessage(`{"endpoint":"https://s3.test","region":"us-east-1","bucket":"photos","access_key_id":"new-test-key","secret_access_key":"new-test-secret","use_path_style":true}`)
		name, baseURL, enabled := "renamed", "https://new-images.test/files", false
		if _, err := f.admin.PatchStorage(t.Context(), f.backend.ID, StoragePatch{Name: &name, BaseURL: &baseURL, Enabled: &enabled, Config: after}); err != nil {
			t.Fatalf("credential/display change with active image refused: %v", err)
		}
		backend, err := f.stores.Find(t.Context(), f.backend.ID)
		if err != nil {
			t.Fatal(err)
		}
		if backend.Name != name || backend.BaseURL != baseURL || backend.Enabled {
			t.Fatal("credential/display change did not persist")
		}
		plain, err := codec.Open(t.Context(), "s3", backend.Config)
		if err != nil {
			t.Fatal(err)
		}
		if string(plain) != string(after) {
			t.Fatal("rotated encrypted configuration did not roundtrip")
		}
		if _, err := f.old.Stat(t.Context(), view.Path+".png"); err != nil {
			t.Fatalf("credential rotation lost existing object: %v", err)
		}
		moved := json.RawMessage(`{"endpoint":"https://s3.test","region":"us-east-1","bucket":"elsewhere","access_key_id":"new-test-key","secret_access_key":"new-test-secret"}`)
		if _, err := f.admin.PatchStorage(t.Context(), f.backend.ID, StoragePatch{Config: moved}); !errors.Is(err, ErrStillReferenced) {
			t.Fatalf("bucket relocation with active image error=%v, want ErrStillReferenced", err)
		}
	})
}
