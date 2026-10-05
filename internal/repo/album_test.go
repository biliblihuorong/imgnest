package repo

import (
	"errors"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// TestAlbumListLiveCounts covers the listing behind GET /api/v1/albums and is
// the regression test for the duplicated image_count output column that broke
// PostgreSQL (SELECT albums.*, COUNT(...) AS image_count).
func TestAlbumListLiveCounts(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture := newImageFixture(t, db, "albums")
		albums, err := NewAlbumRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		blog := model.Album{UserID: fixture.user.ID, Name: "博客", Intro: "文章配图"}
		if err := db.Create(&blog).Error; err != nil {
			t.Fatal(err)
		}
		note := model.Album{UserID: fixture.user.ID, Name: "笔记"}
		if err := db.Create(&note).Error; err != nil {
			t.Fatal(err)
		}
		foreign := model.Album{UserID: 0, Name: "游客的"}
		if err := db.Create(&foreign).Error; err != nil {
			t.Fatal(err)
		}

		assigned := reserveAndCommit(t, fixture, "alb-1", "2026/01/alb-1")
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("UPDATE images SET album_id = ? WHERE id = ?", blog.ID, assigned.ID).Error; err != nil {
			t.Fatal(err)
		}
		trashed := reserveAndCommit(t, fixture, "alb-2", "2026/01/alb-2")
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("UPDATE images SET album_id = ? WHERE id = ?", blog.ID, trashed.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("UPDATE images SET state = ?, deleted_at = ? WHERE id = ?", model.ImageStateTrash, fixedRepoNow, trashed.ID).Error; err != nil {
			t.Fatal(err)
		}

		items, total, err := albums.ListByOwner(t.Context(), fixture.user.ID, 1, 40, "newest", "")
		if err != nil {
			t.Fatal(err)
		}
		if total != 2 || len(items) != 2 {
			t.Fatalf("total=%d items=%d, want the two owned albums", total, len(items))
		}
		if items[0].ID != note.ID || items[1].ID != blog.ID {
			t.Fatalf("newest order = [%d %d], want newest (higher id) first", items[0].ID, items[1].ID)
		}
		// Trashed images leave the live count.
		if items[1].ImageCount != 1 {
			t.Fatalf("live image_count = %d, want 1 (trashed images excluded)", items[1].ImageCount)
		}
		if items[0].ImageCount != 0 {
			t.Fatalf("empty album count = %d, want 0", items[0].ImageCount)
		}

		if most, _, err := albums.ListByOwner(t.Context(), fixture.user.ID, 1, 40, "most", ""); err != nil || most[0].ID != blog.ID {
			t.Fatalf("most order = %+v err=%v, want the populated album first", most, err)
		}
		if least, _, err := albums.ListByOwner(t.Context(), fixture.user.ID, 1, 40, "least", ""); err != nil || least[0].ID != note.ID {
			t.Fatalf("least order = %+v err=%v, want the empty album first", least, err)
		}
		if earliest, _, err := albums.ListByOwner(t.Context(), fixture.user.ID, 1, 40, "earliest", ""); err != nil || earliest[0].ID != blog.ID {
			t.Fatalf("earliest order = %+v err=%v", earliest, err)
		}

		if filtered, total, err := albums.ListByOwner(t.Context(), fixture.user.ID, 1, 40, "newest", "博客"); err != nil || total != 1 || len(filtered) != 1 || filtered[0].Name != "博客" {
			t.Fatalf("keyword filter = %+v total=%d err=%v, want only 博客", filtered, total, err)
		}
		if _, _, err := albums.ListByOwner(t.Context(), fixture.user.ID, 0, 40, "", ""); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("page 0 accepted: %v", err)
		}

		// Other owners' albums stay invisible.
		if foreignItems, _, err := albums.ListByOwner(t.Context(), fixture.user.ID, 1, 40, "newest", "游客"); err != nil || len(foreignItems) != 0 {
			t.Fatalf("foreign album leaked: %+v err=%v", foreignItems, err)
		}
	})
}

func TestAlbumFindAndDelete(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture := newImageFixture(t, db, "album-delete")
		albums, err := NewAlbumRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		blog := model.Album{UserID: fixture.user.ID, Name: "博客"}
		if err := db.Create(&blog).Error; err != nil {
			t.Fatal(err)
		}
		foreign := model.Album{UserID: 0, Name: "游客的"}
		if err := db.Create(&foreign).Error; err != nil {
			t.Fatal(err)
		}

		found, err := albums.FindOwned(t.Context(), fixture.user.ID, blog.ID)
		if err != nil || found.ID != blog.ID || found.Name != "博客" {
			t.Fatalf("FindOwned = %+v err=%v", found, err)
		}
		// An existing album owned by somebody else stays invisible but is
		// reported as forbidden, matching the image service semantics.
		if _, err := albums.FindOwned(t.Context(), fixture.user.ID, foreign.ID); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("foreign album readable: %v", err)
		}
		if _, err := albums.FindOwned(t.Context(), fixture.user.ID, 99999); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("missing album readable: %v", err)
		}

		assigned := reserveAndCommit(t, fixture, "del-1", "2026/01/del-1")
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("UPDATE images SET album_id = ? WHERE id = ?", blog.ID, assigned.ID).Error; err != nil {
			t.Fatal(err)
		}

		if err := albums.DeleteOwned(t.Context(), fixture.user.ID, foreign.ID); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("foreign delete = %v, want forbidden", err)
		}
		if err := albums.DeleteOwned(t.Context(), fixture.user.ID, 99999); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("missing delete = %v, want not found", err)
		}
		if err := albums.DeleteOwned(t.Context(), fixture.user.ID, blog.ID); err != nil {
			t.Fatalf("owned delete failed: %v", err)
		}

		var albumRows, assignedRows int64
		if err := db.Model(&model.Album{}).Where("id = ?", blog.ID).Count(&albumRows).Error; err != nil || albumRows != 0 {
			t.Fatalf("album row survived (count=%d err=%v)", albumRows, err)
		}
		if err := db.Model(&model.Image{}).Where("album_id = ?", blog.ID).Count(&assignedRows).Error; err != nil || assignedRows != 0 {
			t.Fatalf("images kept the deleted album reference (count=%d err=%v)", assignedRows, err)
		}
		var imageRows int64
		if err := db.Model(&model.Image{}).Where("id = ?", assigned.ID).Count(&imageRows).Error; err != nil || imageRows != 1 {
			t.Fatalf("album deletion removed the image itself (count=%d err=%v)", imageRows, err)
		}
	})
}

func TestAlbumCreateUpdateAndLiveCounts(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture := newImageFixture(t, db, "album-write")
		albums, err := NewAlbumRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := albums.Create(t.Context(), model.Album{UserID: fixture.user.ID, Name: "   "}); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("blank album name accepted: %v", err)
		}
		blog, err := albums.Create(t.Context(), model.Album{UserID: fixture.user.ID, Name: "博客", Intro: "旧简介"})
		if err != nil {
			t.Fatal(err)
		}
		if blog.ID == 0 || blog.UserID != fixture.user.ID || blog.ImageCount != 0 || blog.CreatedAt.IsZero() || blog.UpdatedAt.IsZero() {
			t.Fatalf("created album = %+v", blog)
		}
		foreign, err := albums.Create(t.Context(), model.Album{UserID: 0, Name: "游客的"})
		if err != nil {
			t.Fatal(err)
		}

		first := reserveAndCommit(t, fixture, "album-write-1", "2026/01/album-write-1")
		second := reserveAndCommit(t, fixture, "album-write-2", "2026/01/album-write-2")
		if err := db.Exec("UPDATE images SET album_id = ? WHERE id IN (?, ?)", blog.ID, first.ID, second.ID).Error; err != nil {
			t.Fatal(err)
		}

		public := true
		updated, err := albums.Update(t.Context(), fixture.user.ID, blog.ID, map[string]any{
			"name": "新名", "intro": "新简介", "is_public": public, "cover_image_id": first.ID,
		})
		if err != nil {
			t.Fatal(err)
		}
		if updated.Name != "新名" || updated.Intro != "新简介" || !updated.IsPublic || updated.CoverImageID != first.ID {
			t.Fatalf("updated album = %+v", updated)
		}
		// The single record carries the same live count the listing computes.
		if updated.ImageCount != 2 {
			t.Fatalf("live image_count = %d, want 2", updated.ImageCount)
		}
		if _, err := albums.Update(t.Context(), fixture.user.ID, foreign.ID, map[string]any{"name": "x"}); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("foreign album update = %v", err)
		}
		if _, err := albums.Update(t.Context(), fixture.user.ID, 99999, map[string]any{"name": "x"}); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("missing album update = %v", err)
		}

		// Trashing one image drops the live count of the owned album only.
		if err := db.Exec("UPDATE images SET state = ?, deleted_at = ? WHERE id = ?", model.ImageStateTrash, fixedRepoNow, second.ID).Error; err != nil {
			t.Fatal(err)
		}
		reread, err := albums.FindOwned(t.Context(), fixture.user.ID, blog.ID)
		if err != nil {
			t.Fatal(err)
		}
		if reread.ImageCount != 1 {
			t.Fatalf("live image_count after trash = %d, want 1", reread.ImageCount)
		}
	})
}
