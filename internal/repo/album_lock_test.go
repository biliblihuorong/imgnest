package repo

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// An album mutation must not write anything if its shared owner lock cannot
// be acquired. The owner lock serializes these writes with image moves.
func TestAlbumMutationsAbortWhenOwnerLockFails(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		for _, operation := range []string{"update", "delete"} {
			t.Run(operation, func(t *testing.T) {
				fixture := newImageFixture(t, db, "album-lock-"+operation)
				albums, err := NewAlbumRepository(t.Context(), db)
				if err != nil {
					t.Fatal(err)
				}
				album := model.Album{UserID: fixture.user.ID, Name: "Unchanged"}
				if err := db.Create(&album).Error; err != nil {
					t.Fatal(err)
				}
				image := reserveAndCommit(t, fixture, "album-lock-image-"+operation, "lock/"+operation)
				if err := fixture.images.SetAlbum(t.Context(), image.Key, album.ID, fixture.grant); err != nil {
					t.Fatal(err)
				}
				attemptedOwnerLock := false
				const callback = "test:reject_album_owner_lock"
				if err := db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
					if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "users" {
						attemptedOwnerLock = true
						_ = tx.AddError(errors.New("owner lock unavailable"))
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := db.Callback().Query().Remove(callback); err != nil {
						t.Error(err)
					}
				})
				if operation == "update" {
					_, err = albums.Update(t.Context(), fixture.user.ID, album.ID, map[string]any{"name": "Changed", "cover_image_id": image.ID})
				} else {
					err = albums.DeleteOwned(t.Context(), fixture.user.ID, album.ID)
				}
				if !attemptedOwnerLock || err == nil {
					t.Fatalf("mutation did not stop at the unavailable owner lock: attempted=%t error=%v", attemptedOwnerLock, err)
				}
				var stored model.Album
				if err := db.First(&stored, album.ID).Error; err != nil {
					t.Fatal(err)
				}
				if stored.Name != "Unchanged" || stored.CoverImageID != 0 || stored.ImageCount != 1 {
					t.Fatalf("failed mutation changed album: %+v", stored)
				}
				var storedImage model.Image
				if err := db.First(&storedImage, image.ID).Error; err != nil {
					t.Fatal(err)
				}
				if storedImage.AlbumID != album.ID {
					t.Fatalf("failed mutation detached image: album=%d", storedImage.AlbumID)
				}
			})
		}
	})
}

// This executes the repository with PostgreSQL's SQL builder and records its
// emitted statements. It verifies lock ordering, not PostgreSQL concurrency.
func TestAlbumMutationsPostgresLockOwnerBeforeWrites(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, backing *gorm.DB) {
		if backing.Name() != "sqlite" {
			t.Skip("SQL generation uses the isolated SQLite transaction only")
		}
		sqlDB, err := backing.DB()
		if err != nil {
			t.Fatal(err)
		}
		for _, operation := range []string{"update", "delete"} {
			t.Run(operation, func(t *testing.T) {
				recorder := &albumSQLRecorder{Interface: logger.Discard}
				db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, Logger: recorder})
				if err != nil {
					t.Fatal(err)
				}
				if err := db.Callback().Query().After("gorm:query").Register("test:album_query_fixture", func(tx *gorm.DB) {
					switch dest := tx.Statement.Dest.(type) {
					case *model.Album:
						*dest = model.Album{ID: 84, UserID: 42, Name: "Album"}
					case *model.User:
						*dest = model.User{ID: 42}
					}
				}); err != nil {
					t.Fatal(err)
				}
				albums, err := NewAlbumRepository(t.Context(), db)
				if err != nil {
					t.Fatal(err)
				}
				if operation == "update" {
					_, err = albums.Update(t.Context(), 42, 84, map[string]any{"cover_image_id": uint64(23)})
				} else {
					err = albums.DeleteOwned(t.Context(), 42, 84)
				}
				if err != nil {
					t.Fatal(err)
				}
				ownerLocked, sawWrite := false, false
				for _, sql := range recorder.statements {
					if strings.HasPrefix(sql, "SELECT") && strings.Contains(sql, `FROM "users"`) && strings.Contains(sql, "FOR UPDATE") {
						ownerLocked = true
					}
					if strings.HasPrefix(sql, "UPDATE") || strings.HasPrefix(sql, "DELETE") {
						sawWrite = true
						if !ownerLocked {
							t.Fatalf("album/image write preceded owner FOR UPDATE lock: %s; statements=%v", sql, recorder.statements)
						}
					}
				}
				if !ownerLocked || !sawWrite {
					t.Fatalf("missing lock or mutation: %v", recorder.statements)
				}
			})
		}
	})
}

type albumSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *albumSQLRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}
