package repo

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func newRandomLinkFixture(t *testing.T, db *gorm.DB, name string) (imageFixture, *RandomLinkRepository, model.Album) {
	t.Helper()
	fixture := newImageFixture(t, db, name)
	links, err := NewRandomLinkRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	album := model.Album{UserID: fixture.user.ID, Name: name}
	if err := db.Create(&album).Error; err != nil {
		t.Fatal(err)
	}
	return fixture, links, album
}

func TestRandomLinkCRUD(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture, links, album := newRandomLinkFixture(t, db, "link-crud")
		if _, err := links.FindByAlbum(t.Context(), album.ID); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("missing link = %v, want not found", err)
		}
		created, err := links.Create(t.Context(), model.RandomLink{UserID: fixture.user.ID, AlbumID: album.ID, Token: "first-token", Enabled: true})
		if err != nil || created.ID == 0 {
			t.Fatalf("create = %+v err=%v", created, err)
		}
		found, err := links.FindByAlbum(t.Context(), album.ID)
		if err != nil || found.Token != "first-token" || !found.Enabled || found.UserID != fixture.user.ID {
			t.Fatalf("find = %+v err=%v", found, err)
		}
		updated, err := links.Update(t.Context(), album.ID, map[string]any{"enabled": false, "token": "second-token"})
		if err != nil || updated.Enabled || updated.Token != "second-token" || updated.ID != created.ID {
			t.Fatalf("update = %+v err=%v", updated, err)
		}
		if _, err := links.Update(t.Context(), album.ID, map[string]any{"user_id": uint64(9)}); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("foreign column update = %v, want invalid input", err)
		}
		if _, err := links.Update(t.Context(), 99999, map[string]any{"enabled": true}); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("update of missing link = %v, want not found", err)
		}
		for range 2 {
			if err := links.DeleteByAlbum(t.Context(), album.ID); err != nil {
				t.Fatalf("delete: %v", err)
			}
		}
		if _, err := links.FindByAlbum(t.Context(), album.ID); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("deleted link = %v, want not found", err)
		}
	})
}

func TestRandomLinkCreateDuplicate(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture, links, album := newRandomLinkFixture(t, db, "link-dup")
		other := model.Album{UserID: fixture.user.ID, Name: "other"}
		if err := db.Create(&other).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := links.Create(t.Context(), model.RandomLink{UserID: fixture.user.ID, AlbumID: album.ID, Token: "taken", Enabled: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := links.Create(t.Context(), model.RandomLink{UserID: fixture.user.ID, AlbumID: album.ID, Token: "fresh", Enabled: true}); !errors.Is(err, model.ErrRandomLinkExists) {
			t.Fatalf("second link for one album = %v, want exists", err)
		}
		if _, err := links.Create(t.Context(), model.RandomLink{UserID: fixture.user.ID, AlbumID: other.ID, Token: "taken", Enabled: true}); !errors.Is(err, model.ErrRandomLinkExists) {
			t.Fatalf("duplicate token = %v, want exists", err)
		}
	})
}

func TestRandomLinkFindByToken(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture, links, album := newRandomLinkFixture(t, db, "link-token")
		if err := db.Exec("UPDATE users SET public_id = ? WHERE id = ?", "pubid00001", fixture.user.ID).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := links.Create(t.Context(), model.RandomLink{UserID: fixture.user.ID, AlbumID: album.ID, Token: "lookup", Enabled: true}); err != nil {
			t.Fatal(err)
		}
		link, owner, err := links.FindByToken(t.Context(), "lookup")
		if err != nil {
			t.Fatal(err)
		}
		if link.AlbumID != album.ID || owner.ID != fixture.user.ID || owner.Status != model.UserStatusEnabled || owner.PublicID == nil || *owner.PublicID != "pubid00001" {
			t.Fatalf("link=%+v owner=%+v", link, owner)
		}
		if _, _, err := links.FindByToken(t.Context(), "unknown"); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("unknown token = %v, want not found", err)
		}
	})
}

func TestCandidatesOnlyActive(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture, links, album := newRandomLinkFixture(t, db, "cand-active")
		other := model.Album{UserID: fixture.user.ID, Name: "other"}
		if err := db.Create(&other).Error; err != nil {
			t.Fatal(err)
		}
		place := func(key string, albumID uint64, state string) {
			image := reserveAndCommit(t, fixture, key, "2026/10/"+key)
			changes := map[string]any{"album_id": albumID, "state": state}
			if state == model.ImageStateTrash {
				now := time.Now().UTC()
				changes["deleted_at"], changes["purge_at"] = now, now.Add(time.Hour)
			}
			if err := db.Model(&model.Image{}).Where("id = ?", image.ID).Updates(changes).Error; err != nil {
				t.Fatal(err)
			}
		}
		for _, key := range []string{"a1", "a2", "a3"} {
			place(key, album.ID, model.ImageStateActive)
		}
		place("trashed", album.ID, model.ImageStateTrash)
		place("pending", album.ID, model.ImageStatePending)
		place("elsewhere", other.ID, model.ImageStateActive)

		got, err := links.Candidates(t.Context(), album.ID, 5000)
		if err != nil {
			t.Fatal(err)
		}
		paths := map[string]bool{}
		for _, item := range got {
			paths[item.Path] = true
			if item.StorageID != fixture.storage.ID || item.Ext != "jpg" || !item.HasWebP || !item.HasOriginal {
				t.Fatalf("candidate fields = %+v", item)
			}
		}
		if len(got) != 3 || !paths["2026/10/a1"] || !paths["2026/10/a2"] || !paths["2026/10/a3"] {
			t.Fatalf("candidates = %+v", got)
		}
	})
}

func TestCandidatesSkipUnusable(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture, links, album := newRandomLinkFixture(t, db, "cand-unusable")
		for key, flags := range map[string][2]bool{"none": {false, false}, "webp": {false, true}, "orig": {true, false}} {
			image := reserveAndCommit(t, fixture, key, "2026/10/"+key)
			if err := db.Exec("UPDATE images SET album_id = ?, has_original = ?, has_webp = ? WHERE id = ?", album.ID, flags[0], flags[1], image.ID).Error; err != nil {
				t.Fatal(err)
			}
		}
		got, err := links.Candidates(t.Context(), album.ID, 5000)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Fatalf("candidates = %+v, want the two images that still have an object", got)
		}
		for _, item := range got {
			if item.Path == "2026/10/none" {
				t.Fatal("an image without original or WebP became a candidate")
			}
		}
	})
}

func TestCandidatesLimit(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture, links, album := newRandomLinkFixture(t, db, "cand-limit")
		for i := range 7 {
			key := fmt.Sprintf("l%d", i)
			image := reserveAndCommit(t, fixture, key, "2026/10/"+key)
			if err := db.Exec("UPDATE images SET album_id = ? WHERE id = ?", album.ID, image.ID).Error; err != nil {
				t.Fatal(err)
			}
		}
		got, err := links.Candidates(t.Context(), album.ID, 5)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, item := range got {
			seen[item.Path] = true
		}
		if len(got) != 5 || len(seen) != 5 {
			t.Fatalf("limited candidates = %+v", got)
		}
		if _, err := links.Candidates(t.Context(), album.ID, 0); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("zero limit = %v, want invalid input", err)
		}
	})
}

func TestAlbumDeleteRemovesRandomLink(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture, links, album := newRandomLinkFixture(t, db, "link-album-delete")
		albums, err := NewAlbumRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := links.Create(t.Context(), model.RandomLink{UserID: fixture.user.ID, AlbumID: album.ID, Token: "doomed", Enabled: true}); err != nil {
			t.Fatal(err)
		}
		if err := albums.DeleteOwned(t.Context(), fixture.user.ID, album.ID); err != nil {
			t.Fatal(err)
		}
		var rows int64
		if err := db.Model(&model.RandomLink{}).Where("album_id = ?", album.ID).Count(&rows).Error; err != nil || rows != 0 {
			t.Fatalf("link survived its album (count=%d err=%v)", rows, err)
		}
	})
}

func TestEnsurePublicID(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture := newImageFixture(t, db, "public-id")
		other, _, _ := grantFixture(t, db, "public-id-other")
		if err := db.Exec("UPDATE users SET public_id = ? WHERE id = ?", "taken00000", other.ID).Error; err != nil {
			t.Fatal(err)
		}
		calls := 0
		sequence := func(values ...string) func() (string, error) {
			return func() (string, error) {
				value := values[min(calls, len(values)-1)]
				calls++
				return value, nil
			}
		}
		got, err := fixture.users.EnsurePublicID(t.Context(), fixture.user.ID, sequence("taken00000", "taken00000", "fresh00000"))
		if err != nil || got != "fresh00000" || calls != 3 {
			t.Fatalf("ensure = %q err=%v calls=%d", got, err, calls)
		}
		calls = 0
		again, err := fixture.users.EnsurePublicID(t.Context(), fixture.user.ID, sequence("never00000"))
		if err != nil || again != "fresh00000" || calls != 0 {
			t.Fatalf("second ensure = %q err=%v calls=%d", again, err, calls)
		}

		third, _, _ := grantFixture(t, db, "public-id-third")
		calls = 0
		if _, err := fixture.users.EnsurePublicID(t.Context(), third.ID, sequence("taken00000")); err == nil || calls != 5 {
			t.Fatalf("exhausted ensure err=%v calls=%d, want an error after 5 attempts", err, calls)
		}
		var stored *string
		if err := db.Raw("SELECT public_id FROM users WHERE id = ?", third.ID).Scan(&stored).Error; err != nil || stored != nil {
			t.Fatalf("failed ensure stored %v (err=%v)", stored, err)
		}
		if _, err := fixture.users.EnsurePublicID(t.Context(), 99999, sequence("x")); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("missing user = %v, want not found", err)
		}
	})
}
