//go:build integration

package repo

import (
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// TestM5MigrationAlbumCoverClearStoresNull catches numeric zero being written
// into a nullable image foreign key when an album has no selected cover.
func TestM5MigrationAlbumCoverClearStoresNull(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		for _, tc := range []struct {
			name     string
			hasCover bool
		}{
			{name: "edit_coverless_album", hasCover: false},
			{name: "clear_existing_cover", hasCover: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				fixture := newImageFixture(t, db, "m5-cover-"+tc.name)
				albums, err := NewAlbumRepository(t.Context(), db)
				if err != nil {
					t.Fatal(err)
				}
				input := model.Album{UserID: fixture.user.ID, Name: "Album before edit"}
				if tc.hasCover {
					cover := reserveAndCommit(t, fixture, "m5-cover-image", "m5/cover-image")
					input.CoverImageID = cover.ID
				}
				album, err := albums.Create(t.Context(), input)
				if err != nil {
					t.Fatal(err)
				}

				// AlbumService.Update resolves the public sentinel to uint64(0)
				// and passes this map value to the repository unchanged.
				updated, err := albums.Update(t.Context(), fixture.user.ID, album.ID, map[string]any{
					"name": "Album after edit", "cover_image_id": uint64(0),
				})
				if err != nil {
					t.Fatalf("updating album with cover_image_id=0 must succeed: %v", err)
				}
				if updated.Name != "Album after edit" || updated.CoverImageID != 0 {
					t.Fatalf("updated album name=%q cover=%d, want edited name and no cover", updated.Name, updated.CoverImageID)
				}
				var nullCovers int64
				if err := db.Model(&model.Album{}).
					Where("id = ? AND cover_image_id IS NULL", album.ID).
					Count(&nullCovers).Error; err != nil {
					t.Fatal(err)
				}
				if nullCovers != 1 {
					t.Fatal("cleared cover must persist as SQL NULL")
				}
			})
		}
	})
}

// TestM5MigrationMovedImageCanTrashAndRestore catches stale stored album
// counts preventing the lifecycle of an image moved into an empty album.
func TestM5MigrationMovedImageCanTrashAndRestore(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture := newImageFixture(t, db, "m5-move-trash")
		albums, err := NewAlbumRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		album, err := albums.Create(t.Context(), model.Album{UserID: fixture.user.ID, Name: "Move target"})
		if err != nil {
			t.Fatal(err)
		}
		image := reserveAndCommit(t, fixture, "m5-moved-image", "m5/moved-image")
		if image.AlbumID != 0 {
			t.Fatal("fixture upload must start unassigned")
		}
		if err := fixture.images.SetAlbum(t.Context(), image.Key, album.ID, fixture.grant); err != nil {
			t.Fatal(err)
		}
		beforeTrash, err := albums.FindOwned(t.Context(), fixture.user.ID, album.ID)
		if err != nil || beforeTrash.ImageCount != 1 {
			t.Fatalf("moved image must appear in album: count=%d err=%v", beforeTrash.ImageCount, err)
		}

		const trashOp = "m5-trash-moved-image"
		trashed, err := fixture.images.BeginTrash(t.Context(), image.Key, trashOp, fixture.grant, 7)
		if err != nil {
			t.Fatalf("trashing an image moved into an empty album must succeed: %v", err)
		}
		if trashed.State != model.ImageStateTrash || trashed.AlbumID != album.ID {
			t.Fatalf("trash must retain album membership: state=%q album=%d", trashed.State, trashed.AlbumID)
		}
		if err := fixture.images.FinishTrash(t.Context(), image.Key, trashOp); err != nil {
			t.Fatal(err)
		}
		afterTrash, err := albums.FindOwned(t.Context(), fixture.user.ID, album.ID)
		if err != nil || afterTrash.ImageCount != 0 {
			t.Fatalf("trash must leave zero active album members: count=%d err=%v", afterTrash.ImageCount, err)
		}
		if got := usedBytes(t, fixture); got != 0 {
			t.Fatalf("trash must release quota: got %d, want 0", got)
		}

		const restoreOp = "m5-restore-moved-image"
		if _, err := fixture.images.BeginRestore(t.Context(), image.Key, restoreOp, fixture.grant); err != nil {
			t.Fatal(err)
		}
		restored, err := fixture.images.FinishRestore(t.Context(), image.Key, restoreOp, fixture.grant)
		if err != nil {
			t.Fatal(err)
		}
		if err := fixture.images.FinishRestoreCleanup(t.Context(), image.Key, restoreOp); err != nil {
			t.Fatal(err)
		}
		if restored.State != model.ImageStateActive || restored.AlbumID != album.ID {
			t.Fatalf("restore must retain album membership: state=%q album=%d", restored.State, restored.AlbumID)
		}
		afterRestore, err := albums.FindOwned(t.Context(), fixture.user.ID, album.ID)
		if err != nil || afterRestore.ImageCount != 1 {
			t.Fatalf("restore must return one active album member: count=%d err=%v", afterRestore.ImageCount, err)
		}
		if got := usedBytes(t, fixture); got != 160 {
			t.Fatalf("restore must reapply fixture quota: got %d, want 160", got)
		}
	})
}
