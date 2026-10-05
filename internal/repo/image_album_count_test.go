package repo

import (
	"errors"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func TestImageSetAlbumKeepsStoredCountsConsistent(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		for _, tc := range []struct {
			name                                       string
			from, to                                   int
			oldFirst, oldSecond, wantFirst, wantSecond int64
		}{
			{name: "same_album_repairs_drift", from: 1, to: 1, oldFirst: 9, oldSecond: 4, wantFirst: 1, wantSecond: 4},
			{name: "move_between_albums", from: 1, to: 2, oldFirst: 9, oldSecond: 4, wantFirst: 0, wantSecond: 1},
			{name: "repeat_same_album", from: 2, to: 2, oldSecond: 1, wantSecond: 1},
			{name: "remove_from_album", from: 2, to: 0, oldSecond: 9},
			{name: "repeat_unassigned", from: 0, to: 0},
			{name: "assign_unassigned", from: 0, to: 1, wantFirst: 1},
		} {
			t.Run(tc.name, func(t *testing.T) {
				fixture := newImageFixture(t, db, "counts-"+tc.name)
				first := model.Album{UserID: fixture.user.ID, Name: "First", ImageCount: tc.oldFirst}
				second := model.Album{UserID: fixture.user.ID, Name: "Second", ImageCount: tc.oldSecond}
				for _, album := range []*model.Album{&first, &second} {
					if err := db.Create(album).Error; err != nil {
						t.Fatal(err)
					}
				}
				image := reserveAndCommit(t, fixture, "count-"+tc.name, "count/"+tc.name)
				ids := []uint64{0, first.ID, second.ID}
				// Reproduce stored counter drift independently in each case.
				if tc.from != 0 {
					if err := db.Model(&model.Image{}).Where("id = ?", image.ID).
						Update("album_id", ids[tc.from]).Error; err != nil {
						t.Fatal(err)
					}
				}
				if err := fixture.images.SetAlbum(t.Context(), image.Key, ids[tc.to], fixture.grant); err != nil {
					t.Fatal(err)
				}
				assertStoredAlbumCount(t, db, first.ID, tc.wantFirst)
				assertStoredAlbumCount(t, db, second.ID, tc.wantSecond)
			})
		}
	})
}

func TestImageSetAlbumRejectsForeignAlbumWithoutChangingCounts(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture := newImageFixture(t, db, "album-count-owner")
		foreign := model.Album{UserID: 0, Name: "Foreign"}
		if err := db.Create(&foreign).Error; err != nil {
			t.Fatal(err)
		}
		image := reserveAndCommit(t, fixture, "count-foreign", "count/foreign")
		if err := fixture.images.SetAlbum(t.Context(), image.Key, foreign.ID, fixture.grant); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("foreign album assignment error=%v, want forbidden", err)
		}
		var reloaded model.Image
		if err := db.First(&reloaded, image.ID).Error; err != nil {
			t.Fatal(err)
		}
		if reloaded.AlbumID != 0 {
			t.Fatalf("failed assignment changed album to %d", reloaded.AlbumID)
		}
		assertStoredAlbumCount(t, db, foreign.ID, 0)
	})
}

func TestImageSetAlbumConcurrentOppositeMoves(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture := newImageFixture(t, db, "album-count-race")
		first := model.Album{UserID: fixture.user.ID, Name: "First"}
		second := model.Album{UserID: fixture.user.ID, Name: "Second"}
		for _, album := range []*model.Album{&first, &second} {
			if err := db.Create(album).Error; err != nil {
				t.Fatal(err)
			}
		}
		left := reserveAndCommit(t, fixture, "count-left", "count/left")
		right := reserveAndCommit(t, fixture, "count-right", "count/right")
		for _, move := range []struct {
			key     string
			albumID uint64
		}{
			{key: left.Key, albumID: first.ID}, {key: right.Key, albumID: second.ID},
		} {
			if err := fixture.images.SetAlbum(t.Context(), move.key, move.albumID, fixture.grant); err != nil {
				t.Fatal(err)
			}
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		for _, move := range []struct {
			key     string
			albumID uint64
		}{
			{key: left.Key, albumID: second.ID}, {key: right.Key, albumID: first.ID},
		} {
			go func() {
				<-start
				results <- fixture.images.SetAlbum(t.Context(), move.key, move.albumID, fixture.grant)
			}()
		}
		close(start)
		for range 2 {
			if err := <-results; err != nil {
				t.Errorf("concurrent move: %v", err)
			}
		}
		assertStoredAlbumCount(t, db, first.ID, 1)
		assertStoredAlbumCount(t, db, second.ID, 1)
	})
}

func TestImageSetAlbumConcurrentMoveAndTrash(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture := newImageFixture(t, db, "album-move-trash-race")
		first := model.Album{UserID: fixture.user.ID, Name: "First"}
		second := model.Album{UserID: fixture.user.ID, Name: "Second"}
		for _, album := range []*model.Album{&first, &second} {
			if err := db.Create(album).Error; err != nil {
				t.Fatal(err)
			}
		}
		moved := reserveAndCommit(t, fixture, "race-moved", "race/moved")
		trashed := reserveAndCommit(t, fixture, "race-trashed", "race/trashed")
		for _, key := range []string{moved.Key, trashed.Key} {
			if err := fixture.images.SetAlbum(t.Context(), key, first.ID, fixture.grant); err != nil {
				t.Fatal(err)
			}
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		go func() {
			<-start
			results <- fixture.images.SetAlbum(t.Context(), moved.Key, second.ID, fixture.grant)
		}()
		go func() {
			<-start
			_, err := fixture.images.BeginTrash(t.Context(), trashed.Key, "race-trash", fixture.grant, 7)
			results <- err
		}()
		close(start)
		for range 2 {
			if err := <-results; err != nil {
				t.Errorf("concurrent operation: %v", err)
			}
		}
		assertStoredAlbumCount(t, db, first.ID, 0)
		assertStoredAlbumCount(t, db, second.ID, 1)
		if got := usedBytes(t, fixture); got != 160 {
			t.Errorf("used bytes=%d, want 160", got)
		}
	})
}

func TestImageAlbumLifecycleRepairsExistingCountDrift(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture := newImageFixture(t, db, "album-count-repair")
		album := model.Album{UserID: fixture.user.ID, Name: "Old drift"}
		if err := db.Create(&album).Error; err != nil {
			t.Fatal(err)
		}
		image := reserveAndCommit(t, fixture, "count-old", "count/old")
		if err := db.Model(&model.Image{}).Where("id = ?", image.ID).Update("album_id", album.ID).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.images.BeginTrash(t.Context(), image.Key, "count-trash", fixture.grant, 7); err != nil {
			t.Fatalf("old inconsistent count prevented trash: %v", err)
		}
		assertStoredAlbumCount(t, db, album.ID, 0)
		if err := fixture.images.FinishTrash(t.Context(), image.Key, "count-trash"); err != nil {
			t.Fatal(err)
		}
		if err := db.Model(&model.Album{}).Where("id = ?", album.ID).Update("image_count", 9).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.images.BeginRestore(t.Context(), image.Key, "count-restore", fixture.grant); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.images.FinishRestore(t.Context(), image.Key, "count-restore", fixture.grant); err != nil {
			t.Fatal(err)
		}
		assertStoredAlbumCount(t, db, album.ID, 1)
		if _, err := fixture.images.FinishRestore(t.Context(), image.Key, "count-restore", fixture.grant); err != nil {
			t.Fatal(err)
		}
		assertStoredAlbumCount(t, db, album.ID, 1)
	})
}

func assertStoredAlbumCount(t *testing.T, db *gorm.DB, albumID uint64, want int64) {
	t.Helper()
	var album model.Album
	if err := db.First(&album, albumID).Error; err != nil {
		t.Fatal(err)
	}
	if album.ImageCount != want {
		t.Errorf("album %d stored count=%d, want %d", albumID, album.ImageCount, want)
	}
}
