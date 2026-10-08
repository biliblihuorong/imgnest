package repo

import (
	"errors"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func TestImageListAlbumFilter(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture := newImageFixture(t, db, "albumfilter")
		albums, err := NewAlbumRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		blog, err := albums.Create(t.Context(), model.Album{UserID: fixture.user.ID, Name: "博客"})
		if err != nil {
			t.Fatal(err)
		}
		inside := reserveAndCommit(t, fixture, "albumfilter-in", "2026/01/albumfilter-in")
		if err := db.Exec("UPDATE images SET album_id = ? WHERE id = ?", blog.ID, inside.ID).Error; err != nil {
			t.Fatal(err)
		}
		// Uploads without an album keep a NULL reference; the unassigned
		// filter additionally tolerates legacy zero rows.
		loose := reserveAndCommit(t, fixture, "albumfilter-loose", "2026/01/albumfilter-loose")
		outside := reserveAndCommit(t, fixture, "albumfilter-out", "2026/01/albumfilter-out")

		all, total, err := fixture.images.List(t.Context(), model.ImageListFilter{UserID: fixture.user.ID}, 1, 40)
		if err != nil || total != 3 || len(all) != 3 {
			t.Fatalf("unfiltered list total=%d len=%d err=%v", total, len(all), err)
		}
		album := blog.ID
		filtered, total, err := fixture.images.List(t.Context(), model.ImageListFilter{UserID: fixture.user.ID, AlbumID: &album}, 1, 40)
		if err != nil || total != 1 || len(filtered) != 1 || filtered[0].ID != inside.ID {
			t.Fatalf("album list total=%d items=%v err=%v", total, filtered, err)
		}
		unassigned := uint64(0)
		unassignedRows, total, err := fixture.images.List(t.Context(), model.ImageListFilter{UserID: fixture.user.ID, AlbumID: &unassigned}, 1, 40)
		if err != nil || total != 2 || len(unassignedRows) != 2 {
			t.Fatalf("unassigned list total=%d items=%d err=%v", total, len(unassignedRows), err)
		}
		seen := map[uint64]bool{}
		for _, image := range unassignedRows {
			seen[image.ID] = true
		}
		if seen[inside.ID] || !seen[loose.ID] || !seen[outside.ID] {
			t.Fatalf("unassigned filter picked wrong rows: %v", seen)
		}
		missing := uint64(99999)
		if _, total, err := fixture.images.List(t.Context(), model.ImageListFilter{UserID: fixture.user.ID, AlbumID: &missing}, 1, 40); err != nil || total != 0 {
			t.Fatalf("missing album list total=%d err=%v", total, err)
		}
	})
}

func TestImageOwnedActiveImageGuard(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture := newImageFixture(t, db, "coverguard")
		owned := reserveAndCommit(t, fixture, "coverguard-1", "2026/01/coverguard-1")

		image, err := fixture.images.OwnedActiveImage(t.Context(), fixture.user.ID, owned.ID)
		if err != nil || image.ID != owned.ID {
			t.Fatalf("owned active image = %+v err=%v", image, err)
		}
		other, _, _ := grantFixture(t, db, "coverguard-other")
		if _, err := fixture.images.OwnedActiveImage(t.Context(), other.ID, owned.ID); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("foreign cover = %v", err)
		}
		if err := db.Exec("UPDATE images SET state = ?, deleted_at = ? WHERE id = ?", model.ImageStateTrash, fixedRepoNow, owned.ID).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.images.OwnedActiveImage(t.Context(), fixture.user.ID, owned.ID); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("trashed cover = %v", err)
		}
		if _, err := fixture.images.OwnedActiveImage(t.Context(), fixture.user.ID, 99999); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("missing cover = %v", err)
		}
		if _, err := fixture.images.OwnedActiveImage(t.Context(), fixture.user.ID, 0); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("zero cover = %v", err)
		}
	})
}

func TestImageSetAlbumMovesAndClears(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture := newImageFixture(t, db, "albummove")
		albums, err := NewAlbumRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		blog, err := albums.Create(t.Context(), model.Album{UserID: fixture.user.ID, Name: "博客"})
		if err != nil {
			t.Fatal(err)
		}
		image := reserveAndCommit(t, fixture, "albummove-1", "2026/01/albummove-1")

		if err := fixture.images.SetAlbum(t.Context(), image.Key, blog.ID, fixture.grant); err != nil {
			t.Fatal(err)
		}
		reread, err := fixture.images.FindByKey(t.Context(), image.Key)
		if err != nil || reread.AlbumID != blog.ID {
			t.Fatalf("album move = %+v err=%v", reread, err)
		}

		if err := fixture.images.SetAlbum(t.Context(), image.Key, 0, fixture.grant); err != nil {
			t.Fatal(err)
		}
		var nullCount int64
		if err := db.Model(&model.Image{}).Where("id = ? AND album_id IS NULL", image.ID).Count(&nullCount).Error; err != nil || nullCount != 1 {
			t.Fatalf("move out kept a stale album reference (count=%d err=%v)", nullCount, err)
		}

		if err := db.Exec("UPDATE images SET state = ?, deleted_at = ? WHERE id = ?", model.ImageStateTrash, fixedRepoNow, image.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err := fixture.images.SetAlbum(t.Context(), image.Key, blog.ID, fixture.grant); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("trashed image move = %v", err)
		}

		other, _, _ := grantFixture(t, db, "albummove-other")
		foreignGrant := testGrant(other.PasswordHash)
		foreignGrant.UserID = other.ID
		if err := fixture.images.SetAlbum(t.Context(), image.Key, blog.ID, foreignGrant); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("foreign grant move = %v", err)
		}
		if err := fixture.images.SetAlbum(t.Context(), "missing-key", 0, fixture.grant); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("missing image move = %v", err)
		}
	})
}

func TestImageGalleryPagesPublicUploaders(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture := newImageFixture(t, db, "gallery")
		first := reserveAndCommit(t, fixture, "gallery-1", "2026/01/gallery-1")
		if err := db.Exec("UPDATE images SET is_public = ? WHERE id = ?", true, first.ID).Error; err != nil {
			t.Fatal(err)
		}
		second := reserveAndCommit(t, fixture, "gallery-2", "2026/01/gallery-2")
		if err := db.Exec("UPDATE images SET is_public = ? WHERE id = ?", true, second.ID).Error; err != nil {
			t.Fatal(err)
		}
		private := reserveAndCommit(t, fixture, "gallery-private", "2026/01/gallery-private")
		trashed := reserveAndCommit(t, fixture, "gallery-trashed", "2026/01/gallery-trashed")
		if err := db.Exec("UPDATE images SET is_public = ?, state = ?, deleted_at = ? WHERE id = ?", true, model.ImageStateTrash, fixedRepoNow, trashed.ID).Error; err != nil {
			t.Fatal(err)
		}
		guest := model.Image{UserID: 0, PolicyID: fixture.policy.ID, StorageID: fixture.storage.ID, Key: "gallery-guest",
			Path: "2026/01/gallery-guest", Ext: "jpg", MIME: "image/jpeg", State: model.ImageStateActive, IsPublic: true, Frames: 1,
			ObjectManifest: []model.ObjectReceipt{}}
		if err := db.Create(&guest).Error; err != nil {
			t.Fatal(err)
		}

		rows, total, err := fixture.images.ListGallery(t.Context(), 1, 40, false)
		if err != nil {
			t.Fatal(err)
		}
		// Guest uploads join the seeded disabled anchor account and appear
		// under its username once they are public; private and trashed rows
		// never surface.
		if total != 3 || len(rows) != 3 {
			t.Fatalf("gallery total=%d rows=%d, want the two public owned images plus the public guest upload", total, len(rows))
		}
		for _, row := range rows {
			if row.ID == private.ID || row.ID == trashed.ID {
				t.Fatal("private or trashed image leaked into the gallery")
			}
		}
		if rows[0].ID != guest.ID || rows[0].Uploader != "guest" {
			t.Fatalf("guest row = %+v, want uploader guest", rows[0])
		}
		if rows[1].ID != second.ID || rows[1].Uploader != fixture.user.Username || rows[2].ID != first.ID {
			t.Fatalf("gallery order or uploaders = [%+v %+v]", rows[1], rows[2])
		}

		paged, pageTotal, err := fixture.images.ListGallery(t.Context(), 2, 1, false)
		if err != nil || pageTotal != 3 || len(paged) != 1 || paged[0].ID != second.ID {
			t.Fatalf("gallery page 2 total=%d rows=%d err=%v", pageTotal, len(paged), err)
		}
		if _, _, err := fixture.images.ListGallery(t.Context(), 0, 40, false); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("page 0 accepted: %v", err)
		}
	})
}

func TestImageGalleryHidesDisabledAccountsAndPrivateAlbums(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture := newImageFixture(t, db, "galleryscope")
		loose := reserveAndCommit(t, fixture, "scope-loose", "2026/01/scope-loose")
		inPublic := reserveAndCommit(t, fixture, "scope-public", "2026/01/scope-public")
		inPrivate := reserveAndCommit(t, fixture, "scope-private", "2026/01/scope-private")
		shown := model.Album{UserID: fixture.user.ID, Name: "shown", IsPublic: true}
		hidden := model.Album{UserID: fixture.user.ID, Name: "hidden"}
		for _, album := range []*model.Album{&shown, &hidden} {
			if err := db.Create(album).Error; err != nil {
				t.Fatal(err)
			}
		}
		if err := db.Exec("UPDATE images SET is_public = ? WHERE id IN ?", true, []uint64{loose.ID, inPublic.ID, inPrivate.ID}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("UPDATE images SET album_id = ? WHERE id = ?", shown.ID, inPublic.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("UPDATE images SET album_id = ? WHERE id = ?", hidden.ID, inPrivate.ID).Error; err != nil {
			t.Fatal(err)
		}
		rows, total, err := fixture.images.ListGallery(t.Context(), 1, 40, true)
		if err != nil || total != 1 || len(rows) != 1 || rows[0].ID != inPublic.ID {
			t.Fatalf("public-albums gallery total=%d rows=%v err=%v", total, rows, err)
		}
		if _, total, err = fixture.images.ListGallery(t.Context(), 1, 40, false); err != nil || total != 3 {
			t.Fatalf("full gallery total=%d err=%v", total, err)
		}
		if err := db.Exec("UPDATE users SET status = ? WHERE id = ?", model.UserStatusDisabled, fixture.user.ID).Error; err != nil {
			t.Fatal(err)
		}
		if _, total, err = fixture.images.ListGallery(t.Context(), 1, 40, false); err != nil || total != 0 {
			t.Fatalf("disabled account still in gallery: total=%d err=%v", total, err)
		}
	})
}
